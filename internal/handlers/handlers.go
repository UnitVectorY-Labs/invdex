package handlers

import (
	"context"
	"embed"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/UnitVectorY-Labs/invdex/internal/database"
	"github.com/UnitVectorY-Labs/invdex/internal/llm"
	"github.com/UnitVectorY-Labs/invdex/internal/models"
	"github.com/UnitVectorY-Labs/invdex/internal/storage"
)

//go:embed templates/*
var templateFS embed.FS

// Handler holds dependencies for HTTP handlers.
type Handler struct {
	db        *database.DB
	storage   storage.Storage
	llm       *llm.Client
	templates *template.Template
}

// New creates a new Handler with all dependencies.
func New(db *database.DB, store storage.Storage, llmClient *llm.Client) (*Handler, error) {
	funcMap := template.FuncMap{
		"join": strings.Join,
		"contains": func(slice []string, item string) bool {
			for _, s := range slice {
				if s == item {
					return true
				}
			}
			return false
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}

	return &Handler{
		db:        db,
		storage:   store,
		llm:       llmClient,
		templates: tmpl,
	}, nil
}

// RegisterRoutes registers all HTTP routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /", h.handleIndex)
	mux.HandleFunc("GET /items", h.handleListItems)
	mux.HandleFunc("GET /items/{id}", h.handleViewItem)
	mux.HandleFunc("GET /items/new", h.handleNewItem)
	mux.HandleFunc("POST /items", h.handleCreateItem)
	mux.HandleFunc("GET /items/{id}/edit", h.handleEditItem)
	mux.HandleFunc("PUT /items/{id}", h.handleUpdateItem)
	mux.HandleFunc("DELETE /items/{id}", h.handleDeleteItem)
	mux.HandleFunc("GET /tags", h.handleListTags)
	mux.HandleFunc("POST /tags", h.handleCreateTag)
	mux.HandleFunc("DELETE /tags/{id}", h.handleDeleteTag)
	mux.HandleFunc("POST /items/upload", h.handleUploadImage)
	mux.HandleFunc("POST /llm/suggest", h.handleLLMSuggest)
	mux.HandleFunc("POST /llm/chat", h.handleLLMChat)
	mux.HandleFunc("GET /search", h.handleSearch)
}

func (h *Handler) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	items, err := h.db.ListItems(r.Context(), "")
	if err != nil {
		log.Printf("error listing items: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tags, err := h.db.ListTags(r.Context())
	if err != nil {
		log.Printf("error listing tags: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Items": items,
		"Tags":  tags,
	}
	h.render(w, "index.html", data)
}

func (h *Handler) handleListItems(w http.ResponseWriter, r *http.Request) {
	tagFilter := r.URL.Query().Get("tag")
	items, err := h.db.ListItems(r.Context(), tagFilter)
	if err != nil {
		log.Printf("error listing items: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Items": items,
	}
	h.render(w, "items_list.html", data)
}

func (h *Handler) handleViewItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := h.db.GetItem(r.Context(), id)
	if err != nil {
		log.Printf("error getting item: %v", err)
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	// Generate presigned URL for image if present
	if item.ImageURL != "" {
		url, err := h.storage.GetURL(r.Context(), item.ImageURL)
		if err != nil {
			log.Printf("error getting image URL: %v", err)
		} else {
			item.ImageURL = url
		}
	}

	data := map[string]interface{}{
		"Item": item,
	}
	h.render(w, "item_view.html", data)
}

func (h *Handler) handleNewItem(w http.ResponseWriter, r *http.Request) {
	tags, err := h.db.ListTags(r.Context())
	if err != nil {
		log.Printf("error listing tags: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Tags": tags,
	}
	h.render(w, "item_form.html", data)
}

func (h *Handler) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	tags := r.Form["tags"]
	if customTags := r.FormValue("custom_tags"); customTags != "" {
		for _, t := range strings.Split(customTags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	item := &models.Item{
		Title:       r.FormValue("title"),
		Description: r.FormValue("description"),
		ImageURL:    r.FormValue("image_url"),
		Tags:        tags,
	}

	if err := h.db.CreateItem(r.Context(), item); err != nil {
		log.Printf("error creating item: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/items/"+item.ID)
	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) handleEditItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := h.db.GetItem(r.Context(), id)
	if err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	tags, err := h.db.ListTags(r.Context())
	if err != nil {
		log.Printf("error listing tags: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Item": item,
		"Tags": tags,
	}
	h.render(w, "item_edit.html", data)
}

func (h *Handler) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	tags := r.Form["tags"]
	if customTags := r.FormValue("custom_tags"); customTags != "" {
		for _, t := range strings.Split(customTags, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	item := &models.Item{
		ID:          id,
		Title:       r.FormValue("title"),
		Description: r.FormValue("description"),
		ImageURL:    r.FormValue("image_url"),
		Tags:        tags,
	}

	if err := h.db.UpdateItem(r.Context(), item); err != nil {
		log.Printf("error updating item: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/items/"+item.ID)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Get item first to delete associated image
	item, err := h.db.GetItem(r.Context(), id)
	if err == nil && item.ImageURL != "" {
		if delErr := h.storage.Delete(r.Context(), item.ImageURL); delErr != nil {
			log.Printf("error deleting image: %v", delErr)
		}
	}

	if err := h.db.DeleteItem(r.Context(), id); err != nil {
		log.Printf("error deleting item: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.db.ListTags(r.Context())
	if err != nil {
		log.Printf("error listing tags: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Tags": tags,
	}
	h.render(w, "tags.html", data)
}

func (h *Handler) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	tag := &models.Tag{
		Name:     r.FormValue("name"),
		Category: r.FormValue("category"),
	}

	if tag.Name == "" {
		http.Error(w, "Tag name is required", http.StatusBadRequest)
		return
	}

	if err := h.db.CreateTag(r.Context(), tag); err != nil {
		log.Printf("error creating tag: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Return updated tag list
	tags, err := h.db.ListTags(r.Context())
	if err != nil {
		log.Printf("error listing tags: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Tags": tags,
	}
	h.render(w, "tag_list_partial.html", data)
}

func (h *Handler) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.db.DeleteTag(r.Context(), id); err != nil {
		log.Printf("error deleting tag: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Return updated tag list
	tags, err := h.db.ListTags(r.Context())
	if err != nil {
		log.Printf("error listing tags: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Tags": tags,
	}
	h.render(w, "tag_list_partial.html", data)
}

func (h *Handler) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB max
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	key, err := h.storage.Upload(r.Context(), header.Filename, contentType, file)
	if err != nil {
		log.Printf("error uploading file: %v", err)
		http.Error(w, "Upload failed", http.StatusInternalServerError)
		return
	}

	// Return the key as a hidden input for the form
	w.Header().Set("Content-Type", "text/html")
	io.WriteString(w, `<input type="hidden" name="image_url" value="`+key+`">
<div class="upload-success">
	<span class="icon">✓</span> Image uploaded successfully
</div>`)
}

func (h *Handler) handleLLMSuggest(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	if title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	suggestion, err := h.llm.SuggestFromTitle(r.Context(), title)
	if err != nil {
		log.Printf("error getting LLM suggestion: %v", err)
		http.Error(w, "LLM suggestion failed", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Suggestion": suggestion,
	}
	h.render(w, "llm_suggestion.html", data)
}

func (h *Handler) handleLLMChat(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	message := r.FormValue("message")
	if message == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	response, err := h.chatWithLLM(r.Context(), message)
	if err != nil {
		log.Printf("error in LLM chat: %v", err)
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, `<div class="chat-message assistant error">Sorry, I encountered an error. Please try again.</div>`)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	io.WriteString(w, `<div class="chat-message user">`+template.HTMLEscapeString(message)+`</div>
<div class="chat-message assistant">`+template.HTMLEscapeString(response)+`</div>`)
}

func (h *Handler) chatWithLLM(ctx context.Context, message string) (string, error) {
	messages := []llm.ChatMessage{
		{Role: "user", Content: message},
	}

	response, err := h.llm.Chat(ctx, messages)
	if err != nil {
		return "I can help you identify and categorize your collectables. Try describing an item or asking about a specific type of collectable.", err
	}

	return response, nil
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		h.render(w, "items_list.html", map[string]interface{}{"Items": []*models.Item{}})
		return
	}

	items, err := h.db.SearchItems(r.Context(), query)
	if err != nil {
		log.Printf("error searching items: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Items": items,
	}
	h.render(w, "items_list.html", data)
}
