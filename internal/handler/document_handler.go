package handler

import (
    "context"
    "fmt"
    "net/http"
    "log"
    "strconv"
    "net/url"
    "github.com/Azure/azure-storage-blob-go/azblob"
    "gorm.io/gorm"
    "github.com/stefanhall2704/collaborative-doc-editor/internal/model"
)

func DocumentCreateHandler(db *gorm.DB, w http.ResponseWriter, r *http.Request) {
    // Parse form data
    if err := r.ParseForm(); err != nil {
        http.Error(w, "Error parsing form data", http.StatusBadRequest)
        return
    }

    title := r.FormValue("title")
    content := r.FormValue("content")
    ownerIDStr := r.FormValue("user_id")

    log.Printf("Creating document with title: %s", title)

    // Convert ownerIDStr to uint
    ownerID64, err := strconv.ParseUint(ownerIDStr, 10, 32)
    if err != nil {
        log.Printf("Error converting user ID: %v", err)
        http.Error(w, "Invalid user ID format", http.StatusBadRequest)
        return
    }
    ownerID := uint(ownerID64)

    // Setup Azure Blob Storage
    ctx := context.Background()
    accountName := ""
    accountKey := ""
    credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
    if err != nil {
        log.Printf("Error creating Azure storage credential: %s", err)
        return
    }
    pipeline := azblob.NewPipeline(credential, azblob.PipelineOptions{})
    serviceURL, err := url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net/", accountName))
    if err != nil {
        log.Printf("Error parsing service URL: %s", err)
        return
    }
    serviceURLWithPipeline := azblob.NewServiceURL(*serviceURL, pipeline)

    // Create a container URL and blob URL
    containerURL := serviceURLWithPipeline.NewContainerURL("<your_container_name>")
    blobURL := containerURL.NewBlockBlobURL(title) // Using document title as blob name

    // Upload content to Azure Blob Storage
    _, err = azblob.UploadBufferToBlockBlob(ctx, []byte(content), blobURL, azblob.UploadToBlockBlobOptions{})
    if err != nil {
        log.Printf("Error uploading document to Azure Blob Storage: %s", err)
        return
    }

    // Save document metadata to database
    newDoc := model.Document{Title: title, OwnerID: ownerID}
    result := db.Create(&newDoc)
    if result.Error != nil {
        log.Printf("Error creating document metadata: %s", result.Error)
    } else {
        log.Printf("Document metadata created successfully: %v", newDoc)
    }

    // Respond to the client
    if _, err := w.Write([]byte("Document created successfully")); err != nil {
        log.Printf("Error writing response: %v", err)
    }
}
