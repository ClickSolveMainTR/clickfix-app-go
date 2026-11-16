// Command harness, CF-235 keşfi için ortak test düzeneğidir: yerel LiveKit
// (compose.local.yml) için geliştirme token'ı basar, izleyici (temsilci) ve
// yayıncı (müşteri, aday A) sayfalarını sunar. Yalnız yerel geliştirme içindir.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const room = "cf235-spike"

func main() {
	addr := flag.String("addr", "127.0.0.1:7890", "dinleme adresi")
	lkURL := flag.String("livekit", "ws://127.0.0.1:7880", "LiveKit ws adresi")
	lkDist := flag.String("lk-dist", "../../../frontend/node_modules/livekit-client/dist", "livekit-client dist dizini")
	flag.Parse()
	key := env("LIVEKIT_API_KEY", "APIclickfixdev")
	secret := env("LIVEKIT_API_SECRET", "clickfix-local-livekit-secret-change-me")

	// LiveKit'te auto_create kapalı; oda her token isteğinde (idempotent) açılır.
	ensureRoom := func() {
		lkHTTP := strings.Replace(strings.Replace(*lkURL, "wss://", "https://", 1), "ws://", "http://", 1)
		tok, err := mintClaims(key, secret, map[string]any{"iss": key, "sub": "harness", "nbf": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(time.Minute).Unix(), "video": map[string]any{"roomCreate": true}})
		if err != nil {
			log.Printf("oda token: %v", err)
			return
		}
		req, _ := http.NewRequest(http.MethodPost, lkHTTP+"/twirp/livekit.RoomService/CreateRoom", strings.NewReader(`{"name":"`+room+`","empty_timeout":3600}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("oda açılamadı: %v", err)
			return
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			log.Printf("oda açılamadı: HTTP %d", res.StatusCode)
		}
	}
	ensureRoom()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /token", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("identity")
		if id == "" {
			http.Error(w, "identity gerekli", http.StatusBadRequest)
			return
		}
		ensureRoom()
		tok, err := mint(key, secret, id, r.URL.Query().Get("publish") == "1")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_ = json.NewEncoder(w).Encode(map[string]string{"url": *lkURL, "token": tok, "room": room})
	})
	mux.Handle("GET /lk/", http.StripPrefix("/lk/", http.FileServer(http.Dir(*lkDist))))
	web, _ := filepath.Abs("web")
	mux.Handle("GET /", http.FileServer(http.Dir(web)))
	log.Printf("harness http://%s (izleyici: /viewer.html, yayıncı: /publisher.html)", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func mint(key, secret, identity string, publish bool) (string, error) {
	now := time.Now()
	claims := map[string]any{
		"iss": key, "sub": identity, "name": identity,
		"nbf": now.Add(-time.Minute).Unix(), "exp": now.Add(6 * time.Hour).Unix(),
		"video": map[string]any{
			"room": room, "roomJoin": true, "canSubscribe": true,
			"canPublish": publish, "canPublishData": true,
		},
	}
	return mintClaims(key, secret, claims)
}

func mintClaims(key, secret string, claims map[string]any) (string, error) {
	_ = key
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	b, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	p := base64.RawURLEncoding.EncodeToString(b)
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(h + "." + p))
	return h + "." + p + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil)), nil
}
