package handler

import (
	"context"
	"fmt"
	"github.com/Azure/azure-storage-blob-go/azblob"
	"github.com/joho/godotenv"
	"github.com/stefanhall2704/collaborative-doc-editor/internal/model"
	"gorm.io/gorm"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

func DocumentCreateHandler(db *gorm.DB, w http.ResponseWriter, r *http.Request, userID interface{}) {
	ownerID, ok := userID.(uint)
	if !ok {
		log.Printf("ID: %v", ownerID)
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}
	// Parse multipart form data with a max memory of 10MB
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Error parsing multipart form data", http.StatusBadRequest)
		return
	}

	err := godotenv.Load() // This will load the .env file from the same directory
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	accountName := os.Getenv("ACCOUNT_NAME")
	accountKey := os.Getenv("ACCOUNT_KEY")

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

func GetUserFiles(db *gorm.DB, w http.ResponseWriter, r *http.Request, userID interface{}) {
	// Perform type assertion to convert userID to uint
	userIDUint, ok := userID.(uint)
	if !ok {
		log.Printf("ID: %v", userIDUint)
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Retrieve files associated with the user ID from the database
	var documents []model.Document
	if err := db.Where("owner_id = ?", userIDUint).Find(&documents).Error; err != nil {
		http.Error(w, "Error fetching documents", http.StatusInternalServerError)
		return
	}

	log.Printf("Number of documents retrieved: %d", len(documents))

	// Log details of each retrieved document
	for i, doc := range documents {
		log.Printf("Document %d: %v", i+1, doc)
	}

	// Prepare data for rendering the HTML template
	data := struct {
		Documents []model.Document
	}{
		Documents: documents,
	}

	// Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		http.Error(w, "Error getting current working directory", http.StatusInternalServerError)
		return
	}

	// Construct the path to the template file
	templatePath := filepath.Join(cwd, "internal", "handler", "file_list.html")

	// Parse the HTML template
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		http.Error(w, "Error parsing template", http.StatusInternalServerError)
		return
	}

	// Execute the template with data and write the output to the response writer
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Error executing template", http.StatusInternalServerError)
		return
	}
}

func ServeDocumentHandler(db *gorm.DB, w http.ResponseWriter, r *http.Request, docID string) {
	// Load environment variables
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}
	accountName := os.Getenv("ACCOUNT_NAME")
	accountKey := os.Getenv("ACCOUNT_KEY")

	// Convert docID to uint
	docIDUint, err := strconv.ParseUint(docID, 10, 32)
	if err != nil {
		log.Printf("Error converting document ID: %v", err)
		http.Error(w, "Invalid document ID format", http.StatusBadRequest)
		return
	}

	// Retrieve document metadata from database
	var document model.Document
	if err := db.First(&document, docIDUint).Error; err != nil {
		http.Error(w, "Document not found", http.StatusNotFound)
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
	serviceURL, _ := url.Parse(fmt.Sprintf("https://%s.blob.core.windows.net/", accountName))
	serviceURLWithPipeline := azblob.NewServiceURL(*serviceURL, pipeline)
	containerURL := serviceURLWithPipeline.NewContainerURL("collabdocedit")
	blobURL := containerURL.NewBlobURL(document.FileName)

	downloadResponse, err := blobURL.Download(ctx, 0, azblob.CountToEnd, azblob.BlobAccessConditions{}, false, azblob.ClientProvidedKeyOptions{})
	if err != nil {
		log.Printf("Error downloading document from Azure Blob Storage: %s", err)
		return
	}
	bodyStream := downloadResponse.Body(azblob.RetryReaderOptions{MaxRetryRequests: 20})
	defer bodyStream.Close()

	// Serve the file content
	w.Header().Set("Content-Type", "text/plain") // You might want to dynamically set this based on the file type
	if _, err := io.Copy(w, bodyStream); err != nil {
		log.Printf("Error writing file content to response: %v", err)
		http.Error(w, "Error serving file content", http.StatusInternalServerError)
	}
}
