package fakes

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
)

type storedObject struct {
	contentType string
	body        []byte
}

func (s *Server) mountStorage() {
	s.objects = map[string]storedObject{}
	s.live.HandleFunc("POST /storage/v1/object/list/{bucket}", s.storageList)
	s.live.HandleFunc("POST /storage/v1/object/{bucket}/{key...}", s.storagePut)
	s.live.HandleFunc("DELETE /storage/v1/object/{bucket}", s.storageDelete)
	s.live.HandleFunc("GET /storage/v1/object/public/{bucket}/{key...}", s.storagePublic)
}

func foldedStorageRoute(upstream string, r *http.Request) (string, []string, bool) {
	if upstream != "storage" || !strings.HasPrefix(r.URL.Path, "/v1/object/") {
		return "", nil, false
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/object/")
	if rest == "" || strings.HasPrefix(rest, "public/") {
		return "", nil, false
	}
	bucket, _, _ := strings.Cut(rest, "/")
	if bucket == "" {
		return "", nil, false
	}
	full := "/storage" + r.URL.Path
	stable := "/storage/v1/object/" + bucket
	return stable, []string{full, stable}, true
}

func storageOr(route, upstream string, r *http.Request) string {
	if live, ok := storageLiveRoute(upstream, r); ok {
		return live
	}
	return route
}

func storageLiveRoute(upstream string, r *http.Request) (string, bool) {
	if upstream != "storage" || !strings.HasPrefix(r.URL.Path, "/v1/") {
		return "", false
	}
	return "/storage" + r.URL.Path, true
}

func (s *Server) storagePut(w http.ResponseWriter, r *http.Request) {
	if !bearer(r) {
		http.Error(w, "missing bearer", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 3<<20))
	if err != nil || len(body) == 0 {
		http.Error(w, "empty object", http.StatusBadRequest)
		return
	}
	key := r.PathValue("bucket") + "/" + r.PathValue("key")
	s.mu.Lock()
	s.objects[key] = storedObject{contentType: r.Header.Get("Content-Type"), body: body}
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"Key": key})
}

func (s *Server) storageDelete(w http.ResponseWriter, r *http.Request) {
	if !bearer(r) {
		http.Error(w, "missing bearer", http.StatusUnauthorized)
		return
	}
	var body struct {
		Prefixes []string `json:"prefixes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Prefixes) == 0 {
		http.Error(w, "prefixes", http.StatusBadRequest)
		return
	}
	bucket := r.PathValue("bucket")
	s.mu.Lock()
	for key := range s.objects {
		for _, name := range body.Prefixes {
			if key == bucket+"/"+name {
				delete(s.objects, key)
			}
		}
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (s *Server) storagePublic(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("bucket") + "/" + r.PathValue("key")
	s.mu.Lock()
	obj, ok := s.objects[key]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", obj.contentType)
	_, _ = w.Write(obj.body)
}

func (s *Server) storageList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Prefix string `json:"prefix"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
	}
	if !bearer(r) || json.NewDecoder(r.Body).Decode(&body) != nil {
		http.Error(w, "list", http.StatusBadRequest)
		return
	}
	bucket := r.PathValue("bucket") + "/" + body.Prefix + "/"
	type row struct {
		Name string  `json:"name"`
		ID   *string `json:"id"`
	}
	var rows []row
	folders := map[string]bool{}
	s.mu.Lock()
	for key := range s.objects {
		if !strings.HasPrefix(key, bucket) {
			continue
		}
		name := strings.TrimPrefix(key, bucket)
		if strings.Contains(name, "/") {
			folder, _, _ := strings.Cut(name, "/")
			if !folders[folder] {
				rows = append(rows, row{Name: folder})
				folders[folder] = true
			}
			continue
		}
		id := key
		rows = append(rows, row{Name: name, ID: &id})
	}
	s.mu.Unlock()
	slices.SortFunc(rows, func(a, b row) int { return strings.Compare(a.Name, b.Name) })
	start, end := min(body.Offset, len(rows)), min(body.Offset+body.Limit, len(rows))
	_ = json.NewEncoder(w).Encode(rows[start:end])
}

func bearer(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && r.Header.Get("apikey") != ""
}
