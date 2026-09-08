package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	defaultPort              = 3000
	maximumLifetimeMinutes   = 8 * 60
	cleanupInterval          = 30 * time.Second
	maximumFileSize          = 500 * 1024 * 1024
	maximumTotalStorage      = 2 * 1024 * 1024 * 1024
	maximumUploadRequestSize = maximumFileSize + 2*1024*1024
)

//go:embed public
var publicFiles embed.FS

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type metadata struct {
	ID           string `json:"id"`
	OriginalName string `json:"originalName"`
	Description  string `json:"description"`
	Type         string `json:"type"`
	Extension    string `json:"extension"`
	Size         int64  `json:"size"`
	UploadedAt   string `json:"uploadedAt"`
	ExpiresAt    string `json:"expiresAt"`
}

type app struct {
	uploadsDirectory string
	port             int
	uploadMutex      sync.Mutex
	shutdown         func()
}

func main() {
	port := defaultPort
	if value := os.Getenv("PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 || parsed > 65535 {
			log.Fatalf("Ogiltig PORT: %q", value)
		}
		port = parsed
	}

	application := &app{
		uploadsDirectory: "uploads",
		port:             port,
	}

	if err := os.MkdirAll(application.uploadsDirectory, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := application.cleanupExpiredFiles(); err != nil {
		log.Printf("Kunde inte göra första städningen: %v", err)
	}

	frontend, err := fs.Sub(publicFiles, "public")
	if err != nil {
		log.Fatalf("Kunde inte öppna inbäddad frontend: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/info", application.handleInfo)
	mux.HandleFunc("GET /api/files", application.handleListFiles)
	mux.HandleFunc("POST /api/files", application.handleUpload)
	mux.HandleFunc("GET /api/files/{id}/download", application.handleDownload)
	mux.HandleFunc("DELETE /api/files/{id}", application.handleDelete)
	mux.HandleFunc("POST /api/shutdown", application.handleShutdown)
	mux.Handle("/", http.FileServer(http.FS(frontend)))

	server := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", port),
		Handler:           recoverMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	application.shutdown = stop

	go application.cleanupLoop(ctx)

	lanAddress := getLANAddress()
	log.Println("Lokal fildelning är igång:")
	log.Printf("  Den här datorn: http://localhost:%d", port)
	log.Printf("  Lokalt nätverk: http://%s:%d", lanAddress, port)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("Kunde inte stoppa servern rent: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("Panik i %s %s: %v", r.Method, r.URL.Path, recovered)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Något gick fel på servern."})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *app) handleInfo(w http.ResponseWriter, _ *http.Request) {
	lanURL := fmt.Sprintf("http://%s:%d", getLANAddress(), a.port)
	png, err := qrcode.Encode(lanURL, qrcode.Medium, 320)
	if err != nil {
		log.Printf("Kunde inte skapa QR-kod: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Något gick fel på servern."})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"lanUrl":                 lanURL,
		"qrCode":                 "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		"maximumLifetimeMinutes": maximumLifetimeMinutes,
	})
}

func (a *app) handleShutdown(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Servern kan bara stängas från den här datorn."})
		return
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Servern kan bara stängas från den här datorn."})
		return
	}
	if a.shutdown == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Servern kunde inte stängas."})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"message": "Servern stängs."})
	go func() {
		time.Sleep(150 * time.Millisecond)
		a.shutdown()
	}()
}

func (a *app) handleListFiles(w http.ResponseWriter, _ *http.Request) {
	if err := a.cleanupExpiredFiles(); err != nil {
		log.Printf("Städning före listning misslyckades: %v", err)
	}

	files, err := a.readMetadata()
	if err != nil {
		log.Printf("Kunde inte läsa fillistan: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Något gick fel på servern."})
		return
	}

	sort.Slice(files, func(i, j int) bool {
		return parseTime(files[i].UploadedAt).After(parseTime(files[j].UploadedAt))
	})
	writeJSON(w, http.StatusOK, files)
}

func (a *app) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maximumUploadRequestSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "Filen är för stor. Maximal filstorlek är 500 MB."})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Uppladdningen kunde inte läsas."})
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Välj en fil att ladda upp."})
		return
	}
	defer file.Close()

	if header.Size > maximumFileSize {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "Filen är för stor. Maximal filstorlek är 500 MB."})
		return
	}

	requestedLifetime, err := strconv.ParseFloat(r.FormValue("lifetimeMinutes"), 64)
	if err != nil || requestedLifetime <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Livslängden måste vara större än noll."})
		return
	}
	if requestedLifetime > maximumLifetimeMinutes {
		requestedLifetime = maximumLifetimeMinutes
	}

	a.uploadMutex.Lock()
	defer a.uploadMutex.Unlock()

	storageUsed, err := a.storageUsage()
	if err != nil {
		log.Printf("Kunde inte beräkna lagringsutrymme: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Något gick fel på servern."})
		return
	}
	if storageUsed >= maximumTotalStorage || header.Size > maximumTotalStorage-storageUsed {
		writeJSON(w, http.StatusInsufficientStorage, map[string]string{"error": "Det finns inte tillräckligt med ledigt utrymme. Radera någon fil och försök igen."})
		return
	}

	id, err := randomUUID()
	if err != nil {
		log.Printf("Kunde inte skapa fil-id: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Något gick fel på servern."})
		return
	}

	storedPath := a.filePath(id)
	stored, err := os.OpenFile(storedPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		log.Printf("Kunde inte skapa uppladdad fil: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Något gick fel på servern."})
		return
	}

	size, copyErr := io.Copy(stored, io.LimitReader(file, maximumFileSize+1))
	closeErr := stored.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(storedPath)
		if copyErr != nil {
			log.Printf("Kunde inte spara uppladdad fil: %v", copyErr)
		} else {
			log.Printf("Kunde inte stänga uppladdad fil: %v", closeErr)
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Något gick fel på servern."})
		return
	}
	if size > maximumFileSize {
		_ = os.Remove(storedPath)
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "Filen är för stor. Maximal filstorlek är 500 MB."})
		return
	}
	if storageUsed+size > maximumTotalStorage {
		_ = os.Remove(storedPath)
		writeJSON(w, http.StatusInsufficientStorage, map[string]string{"error": "Det finns inte tillräckligt med ledigt utrymme. Radera någon fil och försök igen."})
		return
	}

	originalName := sanitizeFilename(header.Filename)
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(originalName)), ".")
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		if extension != "" {
			contentType = "." + extension
		} else {
			contentType = "Okänd filtyp"
		}
	}

	uploadedAt := time.Now().UTC()
	entry := metadata{
		ID:           id,
		OriginalName: originalName,
		Description:  truncateRunes(strings.TrimSpace(r.FormValue("description")), 500),
		Type:         contentType,
		Extension:    extension,
		Size:         size,
		UploadedAt:   uploadedAt.Format(time.RFC3339Nano),
		ExpiresAt:    uploadedAt.Add(time.Duration(requestedLifetime * float64(time.Minute))).Format(time.RFC3339Nano),
	}

	if err := a.writeMetadata(entry); err != nil {
		_ = os.Remove(storedPath)
		log.Printf("Kunde inte spara metadata: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Något gick fel på servern."})
		return
	}

	writeJSON(w, http.StatusCreated, entry)
}

func (a *app) handleDownload(w http.ResponseWriter, r *http.Request) {
	entry, ok := a.getMetadata(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Filen finns inte längre."})
		return
	}

	contentType := entry.Type
	if strings.HasPrefix(contentType, ".") || contentType == "Okänd filtyp" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": entry.OriginalName}))
	http.ServeFile(w, r, a.filePath(entry.ID))
}

func (a *app) handleDelete(w http.ResponseWriter, r *http.Request) {
	entry, ok := a.getMetadata(r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Filen finns inte längre."})
		return
	}

	if err := a.removeStoredFile(entry.ID); err != nil {
		log.Printf("Kunde inte radera %s: %v", entry.ID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Något gick fel på servern."})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) filePath(id string) string {
	return filepath.Join(a.uploadsDirectory, id+".file")
}

func (a *app) metadataPath(id string) string {
	return filepath.Join(a.uploadsDirectory, id+".json")
}

func (a *app) storageUsage() (int64, error) {
	entries, err := os.ReadDir(a.uploadsDirectory)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".file") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		total += info.Size()
	}
	return total, nil
}

func (a *app) writeMetadata(entry metadata) error {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	temporaryID, err := randomUUID()
	if err != nil {
		return err
	}
	temporary := a.metadataPath(entry.ID) + "." + temporaryID + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, a.metadataPath(entry.ID)); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func (a *app) readMetadata() ([]metadata, error) {
	entries, err := os.ReadDir(a.uploadsDirectory)
	if err != nil {
		return nil, err
	}

	files := make([]metadata, 0, len(entries))
	for _, directoryEntry := range entries {
		if directoryEntry.IsDir() || !strings.HasSuffix(directoryEntry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(directoryEntry.Name(), ".json")
		data, err := os.ReadFile(a.metadataPath(id))
		if err != nil {
			log.Printf("Kunde inte läsa metadata för %s: %v", id, err)
			continue
		}
		var entry metadata
		if err := json.Unmarshal(data, &entry); err != nil {
			log.Printf("Kunde inte tolka metadata för %s: %v", id, err)
			continue
		}
		if entry.ID != id {
			continue
		}
		if _, err := os.Stat(a.filePath(id)); err != nil {
			continue
		}
		files = append(files, entry)
	}
	return files, nil
}

func (a *app) getMetadata(id string) (metadata, bool) {
	if !uuidPattern.MatchString(id) {
		return metadata{}, false
	}
	data, err := os.ReadFile(a.metadataPath(id))
	if err != nil {
		return metadata{}, false
	}
	var entry metadata
	if err := json.Unmarshal(data, &entry); err != nil || entry.ID != id {
		return metadata{}, false
	}
	if _, err := os.Stat(a.filePath(id)); err != nil {
		return metadata{}, false
	}
	if !parseTime(entry.ExpiresAt).After(time.Now()) {
		_ = a.removeStoredFile(id)
		return metadata{}, false
	}
	return entry, true
}

func (a *app) removeStoredFile(id string) error {
	var combined error
	for _, target := range []string{a.filePath(id), a.metadataPath(id)} {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			combined = errors.Join(combined, err)
		}
	}
	return combined
}

func (a *app) cleanupExpiredFiles() error {
	now := time.Now()
	files, err := a.readMetadata()
	if err != nil {
		return err
	}
	for _, entry := range files {
		if !parseTime(entry.ExpiresAt).After(now) {
			if err := a.removeStoredFile(entry.ID); err != nil {
				log.Printf("Kunde inte radera utgången fil %s: %v", entry.ID, err)
			} else {
				log.Printf("Automatiskt raderad: %s", entry.OriginalName)
			}
		}
	}

	directoryEntries, err := os.ReadDir(a.uploadsDirectory)
	if err != nil {
		return err
	}
	for _, entry := range directoryEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		path := filepath.Join(a.uploadsDirectory, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > cleanupInterval*2 {
			_ = os.Remove(path)
		}
	}
	return nil
}

func (a *app) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.cleanupExpiredFiles(); err != nil {
				log.Printf("Automatisk städning misslyckades: %v", err)
			}
		}
	}
}

func getLANAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	var candidates []net.IP
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			var ip net.IP
			switch value := address.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip4 := ip.To4(); ip4 != nil && !ip4.IsLoopback() {
				candidates = append(candidates, ip4)
			}
		}
	}
	for _, candidate := range candidates {
		if candidate.IsPrivate() {
			return candidate.String()
		}
	}
	if len(candidates) > 0 {
		return candidates[0].String()
	}
	return "127.0.0.1"
}

func randomUUID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]), nil
}

func sanitizeFilename(name string) string {
	name = normalizeFilenameEncoding(name)
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	name = truncateRunes(name, 255)
	if name == "" || name == "." {
		return "namnlös-fil"
	}
	return name
}

func normalizeFilenameEncoding(name string) string {
	if utf8.ValidString(name) {
		return name
	}
	runes := make([]rune, 0, len(name))
	for _, b := range []byte(name) {
		runes = append(runes, rune(b))
	}
	return string(runes)
}

func truncateRunes(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum])
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("Kunde inte skriva JSON-svar: %v", err)
	}
}
