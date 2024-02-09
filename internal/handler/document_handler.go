package handler

import (
    "context"
    "fmt"
    "io/ioutil"
    "os"
    "net/http"
    "log"
    "strconv"
    "net/url"
    "github.com/Azure/azure-storage-blob-go/azblob"
    "gorm.io/gorm"
    "github.com/joho/godotenv"
    "github.com/stefanhall2704/collaborative-doc-editor/internal/model"
)


func DocumentCreateHandler(db *gorm.DB, w http.ResponseWriter, r *http.Request) {
    // Parse multipart form data with a max memory of 10MB
    if err := r.ParseMultipartForm(10 << 20); err != nil {
        http.Error(w, "Error parsing multipart form data", http.StatusBadRequest)
        return
    }

    ownerIDStr := r.FormValue("user_id")
    err := godotenv.Load() // This will load the .env file from the same directory
    if err != nil {
        log.Fatalf("Error loading .env file: %v", err)
    }
    accountName := os.Getenv("ACCOUNT_NAME")
    accountKey := os.Getenv("ACCOUNT_KEY")

    // Convert ownerIDStr to uint
    ownerID64, err := strconv.ParseUint(ownerIDStr, 10, 32)
    if err != nil {
        log.Printf("Error converting user ID: %v", err)
        http.Error(w, "Invalid user ID format", http.StatusBadRequest)
        return
    }
    ownerID := uint(ownerID64)

    // Retrieve the file from the posted form-data
    file, header, err := r.FormFile("attachment")
    if err != nil {
        log.Printf("Error retrieving the file from form data: %v", err)
        http.Error(w, "Invalid file upload", http.StatusBadRequest)
        return
    }
    defer file.Close()
    fileName := header.Filename

    // Read the contents of the file
    fileBytes, err := ioutil.ReadAll(file)
    if err != nil {
        log.Printf("Error reading the file contents: %v", err)
        http.Error(w, "Error reading the file", http.StatusInternalServerError)
        return
    }

    // Setup Azure Blob Storage
    ctx := context.Background()
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

    // Create a container URL and blob URL using the file name
    containerURL := serviceURLWithPipeline.NewContainerURL("collabdocedit")
    blobURL := containerURL.NewBlockBlobURL(header.Filename) // Using file name as blob name
    contentType := header.Header.Get("Content-Type")
    // Set the Content-Type for the blob
    blobHTTPHeaders := azblob.BlobHTTPHeaders{ContentType: contentType}

    // Upload the file to Azure Blob Storage
    _, err = azblob.UploadBufferToBlockBlob(ctx, fileBytes, blobURL, azblob.UploadToBlockBlobOptions{
        BlobHTTPHeaders: blobHTTPHeaders,
    })
    if err != nil {
        log.Printf("Error uploading file to Azure Blob Storage: %s", err)
        return
    }

    // Save document metadata to database
    newDoc := model.Document{FileName: fileName, ContentType: contentType, OwnerID: ownerID}
    result := db.Create(&newDoc)
    if result.Error != nil {
        log.Printf("Error creating document metadata: %s", result.Error)
    } else {
        log.Printf("Document metadata created successfully: %v", newDoc)
    }

    // Respond to the client
    if _, err := w.Write([]byte("Document created successfully with attachment")); err != nil {
        log.Printf("Error writing response: %v", err)
    }
}
