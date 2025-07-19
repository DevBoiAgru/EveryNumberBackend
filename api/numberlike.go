// This backend would look and perform wayyyy nicer if I didn't have to rely on vercel
// for hosting the api. I'd self host it but unfortunately cloudflare requires a domain
// and I'm not in the mood for getting ddos-ed by a random child.

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

	// Use a limited reader so that someone doesnt send a 50TB payload and we load all of
	// that in memory with io.ReadAll
	const maxBodySize = 32 // Max 32 bytes
	limitedReader := io.LimitReader(r.Body, maxBodySize)
	numRaw, err := io.ReadAll(limitedReader)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Printf("[ERROR] Error while reading request body: %s\n", err)
		return
	}
	defer r.Body.Close()

	num, err := strconv.ParseInt(string(numRaw), 10, 64)
	if err != nil {
		http.Error(w, "Invalid number", http.StatusBadRequest)
		log.Printf("[WARN] Invalid number inputted: %s\n", numRaw)
		return
	}

	addLike(num)

	// Get number of likes for surrounding numbers to return
	surrLikes := GetSurroundingLikes(num)
	likesJSON, err := json.Marshal(surrLikes)

	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		log.Println("[ERROR] [POST]  Failed to marshal surrounding likes map to json.")
		return
	}

	w.WriteHeader(http.StatusCreated)
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
		log.Printf("[ERROR] [REDIS] Error while incrementing likes for %d: %v\n", number, err)
	}
}

func GetSurroundingLikes(center int64) map[int64]uint32 {
	result := make(map[int64]uint32)
	const radius int64 = 10

	reqNums := make([]string, 0, 21) // 10 above, 10 below, 1 middle
	nums := make([]int64, 0, 21)     // For keeping track of which number we are getting the number of likes for

	for i := int64(-radius); i <= radius; i++ {
		n := center + i
		if (i < 0 && n > center) || (i > 0 && n < center) {
			// Overflow / underflow happened, skip
			continue
		}
		reqNums = append(reqNums, fmt.Sprintf("like:%d", n))
		nums = append(nums, n)
	}

	// Get number of likes for all required numbers at once
	likes := redisClient.MGet(ctx, reqNums...)

	res, err := likes.Result()
	if err != nil {
		log.Printf("[ERROR] [REDIS] Error while getting likes around %d: %v\n", center, err)
	}
	for i, val := range res {
		if val == nil {
			result[nums[i]] = 0
		} else {

			// Parse value into a uint32
			strVal, ok := val.(string)
			if !ok {
				log.Printf("[ERROR] [REDIS] Got unexpected type %s\n", val)
				continue
			}

			// Convert that string into a number
			likeNum, err := strconv.ParseInt(strVal, 10, 32)
			if err != nil {
				log.Printf("Key %s has non-numeric value: %s\n", reqNums[i], strVal)
				continue
			}
			result[nums[i]] = uint32(likeNum)
		}
	}

	return result
}
