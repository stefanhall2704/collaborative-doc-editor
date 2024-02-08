package handler

import (
    "net/http"
    "log"
    "gorm.io/gorm"
    "strconv"
    "github.com/stefanhall2704/collaborative-doc-editor/internal/model"
)

func DocumentCreateHandler(db *gorm.DB, w http.ResponseWriter, r *http.Request) {
    // Check for errors when parsing form data
    if err := r.ParseForm(); err != nil {
        http.Error(w, "Error parsing form data", http.StatusBadRequest)
        return
    }

    title := r.FormValue("title")
    content := r.FormValue("content")
    ownerIDStr := r.FormValue("user_id") // Replace with actual logic to retrieve the authenticated user's ID

    log.Printf("Creating document with title: %s, content: %s", title, content)
    
    ownerID64, err := strconv.ParseUint(ownerIDStr, 10, 32)
    if err != nil {
        log.Printf("Error converting user ID: %v", err)
        http.Error(w, "Invalid user ID format", http.StatusBadRequest)
        return // Make sure to return after sending the error response
    }
    
    ownerID := uint(ownerID64) // Convert uint64 to uint

    newDoc := model.Document{Title: title, Content: content, OwnerID: ownerID}
    result := db.Create(&newDoc)
    if result.Error != nil {
        log.Printf("Error creating document: %s", result.Error)
    } else {
        log.Printf("Document created successfully: %v", newDoc)
    }

    // Check for errors when writing the response
    if _, err := w.Write([]byte("Document created successfully")); err != nil {
        log.Printf("Error writing response: %v", err)
    }
}

