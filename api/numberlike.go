package handler

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
)

func NumberLike(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		handlePost(w, r)
	case http.MethodGet:
		handleGet(w, r)
	case http.MethodOptions:
		handleOptions(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
}

func handlePost(w http.ResponseWriter, r *http.Request) {

	numRaw, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	num, err := strconv.ParseInt(string(numRaw), 10, 64)
	if err != nil {
		http.Error(w, "Invalid number", http.StatusBadRequest)
		log.Println("[WARN] Invalid number inputted (string failed to convert to integer)")
		return
	}

	addLike(num)
	log.Printf("[INFO] Liked number %d\n", num)

	// Get number of likes for surrounding numbers to return
	surrLikes := GetSurroundingLikes(num)
	likesJSON, err := json.Marshal(surrLikes)

	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Println("[ERROR] [POST]  Failed to marshal surrounding likes map to json.")
		return
	}

	fmt.Printf("%s\n", string(likesJSON))
	w.WriteHeader(http.StatusOK)
	w.Write(likesJSON)
}

func handleGet(w http.ResponseWriter, r *http.Request) {

	numStr := r.URL.Query().Get("n")
	if numStr == "" {
		http.Error(w, "No number provided (query parameter 'n')", http.StatusBadRequest)
		return
	}

	num, err := strconv.ParseInt(string(numStr), 10, 64)
	if err != nil {
		http.Error(w, "Invalid number.", http.StatusBadRequest)
		log.Println("[WARN] Invalid number inputted (string failed to convert to integer)")
		return
	}

	// Get number of likes for surrounding numbers to return
	surrLikes := GetSurroundingLikes(num)
	likesJSON, err := json.Marshal(surrLikes)

	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Println("[ERROR] [GET] Failed to marshal surrounding likes map to json.")
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write(likesJSON)
}

func handleOptions(w http.ResponseWriter, _ *http.Request) {
	// Set CORS headers

	// Let vercel handle origins. 
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Set("Access-Control-Max-Age", "86400") // 1 day

	// Also include the standard Allow header
	w.Header().Set("Allow", "GET, POST, OPTIONS")

	// No content needed
	w.WriteHeader(http.StatusNoContent)
}

// Functions and variables used by the API, can't be seperated into another file due to some vercel reason

var (
	redisClient = redis.NewClient(&redis.Options{
		Addr:      os.Getenv("REDIS_ADDR"),
		Password:  os.Getenv("REDIS_TOKEN"),
		TLSConfig: &tls.Config{},
	})
	ctx = context.Background()
)

// Functions used to retrieve / add likes

func addLike(number int64) {
	key := fmt.Sprintf("like:%d", number)
	err := redisClient.Incr(ctx, key).Err()
	if err != nil {
		log.Println("Redis Incr error:", err)
	}
}

func getLikes(number int64) uint32 {
	key := fmt.Sprintf("like:%d", number)
	val, err := redisClient.Get(ctx, key).Uint64()
	if err == redis.Nil {
		return 0
	} else if err != nil {
		log.Println("Redis Get error:", err)
		return 0
	}
	return uint32(val)
}

func GetSurroundingLikes(center int64) map[int64]uint32 {
	result := make(map[int64]uint32)
	const radius int64 = 10

	for i := int64(-radius); i <= radius; i++ {
		n := center + i
		if (i < 0 && n > center) || (i > 0 && n < center) {
			// Overflow / underflow happened, skip
			continue
		}
		result[n] = getLikes(n)
	}

	return result
}
