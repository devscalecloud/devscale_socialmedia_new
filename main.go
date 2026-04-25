package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type User struct {
	Username string
	Password string
	Name     string
}

type Session struct {
	Username  string
	ExpiresAt time.Time
}

type Post struct {
	ID        int
	Author    string
	Content   string
	CreatedAt time.Time
}

type DashboardData struct {
	CurrentUser string
	Posts       []Post
	Error       string
	ServerIP    string
	Hostname    string
}

type LoginData struct {
	Error    string
	ServerIP string
	Hostname string
}

var (
	users = map[string]User{
		"alex": {
			Username: "alex",
			Password: "password123",
			Name:     "Alex Morgan",
		},
		"sam": {
			Username: "sam",
			Password: "secure456",
			Name:     "Sam Carter",
		},
	}

	sessions = map[string]Session{}
	posts    = []Post{}
	nextID   = 1
	mu       sync.RWMutex
	tmpl     *template.Template
	hostname = "Unknown"
	serverIP = "Unavailable"
)

func resolveServerInfo() (string, string) {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "Unknown"
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return host, "Unavailable"
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			ip = ip.To4()
			if ip == nil {
				continue
			}

			return host, ip.String()
		}
	}

	return host, "Unavailable"
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func getSessionUser(r *http.Request) (User, bool) {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return User{}, false
	}

	mu.RLock()
	session, ok := sessions[cookie.Value]
	mu.RUnlock()
	if !ok || time.Now().After(session.ExpiresAt) {
		return User{}, false
	}

	mu.RLock()
	user, exists := users[session.Username]
	mu.RUnlock()
	return user, exists
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if _, ok := getSessionUser(r); ok {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "login.html", LoginData{ServerIP: serverIP, Hostname: hostname}); err != nil {
			log.Printf("login template error: %v", err)
		}
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	mu.RLock()
	user, ok := users[username]
	mu.RUnlock()
	if !ok || subtle.ConstantTimeCompare([]byte(user.Password), []byte(password)) != 1 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.ExecuteTemplate(w, "login.html", LoginData{
			Error:    "Invalid username or password.",
			ServerIP: serverIP,
			Hostname: hostname,
		}); err != nil {
			log.Printf("login template error: %v", err)
		}
		return
	}

	token, err := generateToken()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	mu.Lock()
	sessions[token] = Session{Username: username, ExpiresAt: time.Now().Add(24 * time.Hour)}
	mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getSessionUser(r)
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	mu.RLock()
	feed := make([]Post, len(posts))
	copy(feed, posts)
	mu.RUnlock()

	sort.Slice(feed, func(i, j int) bool {
		return feed[i].CreatedAt.After(feed[j].CreatedAt)
	})

	data := DashboardData{
		CurrentUser: user.Name,
		Posts:       feed,
		Error:       r.URL.Query().Get("error"),
		ServerIP:    serverIP,
		Hostname:    hostname,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		log.Printf("dashboard template error: %v", err)
	}
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := getSessionUser(r)
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/dashboard?error=Unable+to+post+right+now", http.StatusSeeOther)
		return
	}

	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" {
		http.Redirect(w, r, "/dashboard?error=Post+cannot+be+empty", http.StatusSeeOther)
		return
	}
	if len(content) > 280 {
		http.Redirect(w, r, "/dashboard?error=Post+must+be+280+characters+or+less", http.StatusSeeOther)
		return
	}

	mu.Lock()
	posts = append(posts, Post{
		ID:        nextID,
		Author:    user.Name,
		Content:   content,
		CreatedAt: time.Now(),
	})
	nextID++
	mu.Unlock()

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		mu.Lock()
		delete(sessions, cookie.Value)
		mu.Unlock()
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func main() {
	var err error
	hostname, serverIP = resolveServerInfo()

	tmpl, err = template.New("").Funcs(template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("02 Jan 2006, 15:04")
		},
	}).ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}

	// Seed a few demo posts for a realistic feed on first login.
	mu.Lock()
	if len(posts) == 0 {
		posts = append(posts,
			Post{ID: nextID, Author: "Alex Morgan", Content: "Building in public today. Shipping this social dashboard with Go + HTML only.", CreatedAt: time.Now().Add(-90 * time.Minute)},
		)
		nextID++
		posts = append(posts,
			Post{ID: nextID, Author: "Sam Carter", Content: "Reminder: small consistent progress beats giant plans. What are you shipping this week?", CreatedAt: time.Now().Add(-35 * time.Minute)},
		)
		nextID++
	}
	mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/", loginHandler)
	mux.HandleFunc("/dashboard", dashboardHandler)
	mux.HandleFunc("/post", postHandler)
	mux.HandleFunc("/logout", logoutHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := ":" + port
	fmt.Println("============================================")
	fmt.Println("  DevScale Social Media App")
	fmt.Println("============================================")
	fmt.Printf("  Server running at http://0.0.0.0%s\n", addr)
	fmt.Printf("  Hostname: %s\n", hostname)
	fmt.Printf("  IP Address: %s\n", serverIP)
	fmt.Println("  Open your browser: http://localhost:" + port)
	fmt.Println("  Demo login: alex / password123")
	fmt.Println("============================================")
	log.Fatal(http.ListenAndServe(addr, mux))
}
