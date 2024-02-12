package main

import (
	"fmt"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/stefanhall2704/collaborative-doc-editor/internal/db"
  "github.com/gorilla/websocket"
	"github.com/stefanhall2704/collaborative-doc-editor/internal/handler"
	"github.com/stefanhall2704/collaborative-doc-editor/internal/model"
)

func enableCors(w *http.ResponseWriter) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	(*w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}

var store = sessions.NewCookieStore([]byte("secret"))

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins
}
// A global list to keep track of all active connections
var clients = make(map[*websocket.Conn]bool)

// A channel to broadcast messages to all clients
var broadcast = make(chan []byte)

var docSessions = make(map[string][]*websocket.Conn)
var mutex = &sync.Mutex{} // Mutex to protect access to docSessions


func wsEndpoint(w http.ResponseWriter, r *http.Request) {
    ws, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println(err)
        return
    }
    defer ws.Close() // Ensure WebSocket is closed on function exit

    docID := r.URL.Query().Get("docID")
    if docID == "" || docID == "null" {
        log.Println("docID is missing or null")
        return
    }

    // Register connection
    mutex.Lock()
    docSessions[docID] = append(docSessions[docID], ws)
    mutex.Unlock()

    // Setup a cleanup routine to run when the function exits
    defer func() {
        mutex.Lock()
        defer mutex.Unlock() // Ensure mutex is unlocked after cleanup
        // Find and remove the closed connection from its session
        for i, conn := range docSessions[docID] {
            if conn == ws {
                // Remove connection by reslicing
                docSessions[docID] = append(docSessions[docID][:i], docSessions[docID][i+1:]...)
                break
            }
        }
        // Optional: Delete the docID key if no connections are left to avoid memory leak
        if len(docSessions[docID]) == 0 {
            delete(docSessions, docID)
        }
    }()

    for {
        _, message, err := ws.ReadMessage()
        if err != nil {
            log.Println("read error:", err)
            break // Exit the loop and trigger cleanup on function exit
        }

        mutex.Lock()
        // Broadcast message to clients editing the same document
        for _, conn := range docSessions[docID] {
            if conn != ws { // Don't send the message back to its sender
                if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
                    log.Println("write error:", err)
                    // No need for additional logic here as cleanup is handled by defer
                }
            }
        }
        mutex.Unlock()
    }

    // No need to explicitly call cleanup logic here as defer will handle it
}


func handleMessages() {
    for {
        msg := <-broadcast // Receive message
        for client := range clients { // Send it to all connected clients
            err := client.WriteMessage(websocket.TextMessage, msg)
            if err != nil {
                log.Printf("client write error: %v", err)
                client.Close()
                delete(clients, client)
            } else {
            	log.Printf("broadcast to a client")
            }
        }
    }
}


// Registration Handler
func registerHandler(w http.ResponseWriter, r *http.Request) {
	// Parse form data

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form data", http.StatusBadRequest)
		return
	}
	// Get username and password from form
	username := r.Form.Get("username")
	password := r.Form.Get("password")
	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Error hashing password", http.StatusInternalServerError)
		return
	}
	// Save user to the database
	user := model.User{
		Username:     username,
		PasswordHash: string(hashedPassword),
		Email:        r.Form.Get("email"),
	}

	database := db.ConnectDatabase()
	if err := database.Create(&user).Error; err != nil {
		http.Error(w, "Error creating user", http.StatusInternalServerError)
		return
	}
	// Redirect or respond with success message
}

// Login Handler
func loginHandler(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form data", http.StatusBadRequest)
		return
	}
	// Get username and password from form
	username := r.Form.Get("username")
	password := r.Form.Get("password")
	// Query user from the database by username
	var user model.User

	database := db.ConnectDatabase()
	if err := database.Where("username = ?", username).First(&user).Error; err != nil {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}
	// Compare hashed password with provided password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}
	// Create session
	session, err := store.Get(r, "session-name")
	if err != nil {
		http.Error(w, "Error getting session", http.StatusInternalServerError)
		return
	}
	session.Values["user"] = username
	session.Values["userID"] = user.ID // Store user ID as well

	// Save the session
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Error saving session", http.StatusInternalServerError)
		return
	}

	// Log session creation
	fmt.Println("Session created successfully")

	// Redirect to the home page or any other desired page
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout Handler
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	// Clear session
	session, _ := store.Get(r, "session-name")
	delete(session.Values, "user")
	if err := session.Save(r, w); err != nil {
		http.Error(w, "Error saving session", http.StatusInternalServerError)
		return
	}
	// Redirect to login page
	http.Redirect(w, r, "/login", http.StatusFound)
}

// Authentication Middleware
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Retrieve session
		session, err := store.Get(r, "session-name")
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Check session for authentication
		if _, ok := session.Values["user"]; !ok {
			// Redirect to login page if user is not authenticated
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}

// Protected Handler
func protectedHandler(w http.ResponseWriter, r *http.Request) {
	// You can put any logic here for handling protected requests
	fmt.Fprintln(w, "This is a protected route.")
}
func main() {
	database := db.ConnectDatabase()
	// Websockets endpoints
	http.HandleFunc("/ws", wsEndpoint)
	go handleMessages()
	// Apply middleware to protected routes
	http.HandleFunc("/register", renderRegistrationPage)
	http.HandleFunc("/register/process", registerHandler)
	http.HandleFunc("/login", renderLoginPage)
	http.HandleFunc("/login/process", loginHandler)
	http.Handle("/protected", authMiddleware(http.HandlerFunc(protectedHandler)))
	//getUserFiles
	// Apply middleware to other protected routes
	http.Handle("/", logRequest(authMiddleware(http.HandlerFunc(documentEditing))))
	http.Handle("/logout", logRequest(authMiddleware(http.HandlerFunc(logoutHandler))))
	http.Handle("/documents/create", logRequest(authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enableCors(&w)
		if r.Method == "OPTIONS" {
			return // Handle preflight request
		}

		session, _ := store.Get(r, "session-name")
		userID := session.Values["userID"]
		handler.DocumentCreateHandler(database, w, r, userID)
	}))))
	http.Handle("/documents/get", logRequest(authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enableCors(&w)
		if r.Method != "GET" {
			log.Println("Wrong Protocol, use the GET protocol")
			return // Handle preflight request
		}
		session, _ := store.Get(r, "session-name")
		userID := session.Values["userID"]
		handler.GetUserFiles(database, w, r, userID)
	}))))

	http.Handle("/documents/get/shared", logRequest(authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enableCors(&w)
		if r.Method != "GET" {
			log.Println("Wrong Protocol, use the GET protocol")
			return // Handle preflight request
		}
		handler.GetSharedFiles(database, w, r)
	}))))
	http.Handle("/documents/serve", logRequest(authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enableCors(&w)
		if r.Method != "GET" {
			log.Println("Only GET requests are supported for document serving")
			return
		}
		// Assuming you're passing a document ID as a query parameter
		docID := r.URL.Query().Get("id")
		if docID == "" {
			http.Error(w, "Document ID is required", http.StatusBadRequest)
			return
		}
		handler.ServeDocumentHandler(database, w, r, docID)
	}))))

	log.Println("Starting server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
		next.ServeHTTP(w, r)
	})
}

func documentEditing(w http.ResponseWriter, r *http.Request) {
	cwd, _ := os.Getwd() // Gets the current working directory
	// Adjust the template path to be relative to the project root
	templatePath := filepath.Join(cwd, "web", "templates", "main.html")

	tmpl, err := template.ParseFiles(templatePath)

	if err != nil {
		log.Printf("Error parsing template: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, nil); err != nil {
		log.Printf("Error executing template: %s", err)
		// Error handling after attempting to write to the response might be limited
	}
}

// renderRegistrationPage renders the registration page
func renderRegistrationPage(w http.ResponseWriter, r *http.Request) {
	cwd, _ := os.Getwd() // Gets the current working directory
	// Adjust the template path to be relative to the project root
	templatePath := filepath.Join(cwd, "web", "templates", "register.html")

	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		log.Printf("Error parsing template: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, nil); err != nil {
		log.Printf("Error executing template: %s", err)
		// Error handling after attempting to write to the response might be limited
	}
}
func renderLoginPage(w http.ResponseWriter, r *http.Request) {
	cwd, _ := os.Getwd() // Gets the current working directory
	// Adjust the template path to be relative to the project root
	templatePath := filepath.Join(cwd, "web", "templates", "login.html")

	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		log.Printf("Error parsing template: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, nil); err != nil {
		log.Printf("Error executing template: %s", err)
	}
}
