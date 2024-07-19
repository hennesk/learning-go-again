package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/redis/go-redis/v9"
)

const DEFAULT_REDIS_URL string = "redis://localhost:6379/0"
const DEFAULT_TTL int = 24 * 60 * 60
const TTL_MAX int = 7 * 24 * 60 * 60

var log = slog.New(slog.NewJSONHandler(os.Stdout, nil))
var redisClient *redis.Client
var ctx context.Context = context.Background()

type RedisInput struct {
	User     string
	UserType string
	Action   string
	Ttl      int
}

type RedisOutput struct {
	User     string `json:"user"`
	UserType string `json:"userType"`
	Action   string `json:"action"`
}

type ErrorResponse struct {
	StatusCode   int    `json:"statusCode"`
	ErrorMessage string `json:"errorMessage"`
}

func getRedisClient() *redis.Client {
	redisUrl := os.Getenv("REDIS_URL")
	if redisUrl == "" {
		redisUrl = DEFAULT_REDIS_URL
	}
	opt, err := redis.ParseURL(redisUrl)
	if err != nil {
		log.Warn("Could not parse the given Redis URL. Trying the default value.")
		opt, err = redis.ParseURL(DEFAULT_REDIS_URL)
		if err != nil {
			log.Error("Something truly crazy has occurred. Could not parse the default Redis URL. Exiting.")
			os.Exit(1)
		}
	}
	client := redis.NewClient(opt)
	err = client.Ping(context.Background()).Err()
	if err != nil {
		log.Error("Could not connect to Redis. Exiting.")
		os.Exit(1)
	}
	redisClient = client
	return client
}

func getRedisEntryBySlug(slug string, client *redis.Client) (RedisOutput, error) {
	value, err := client.HGetAll(ctx, slug).Result()
	mappedValue := RedisOutput{User: value["user"], UserType: value["userType"], Action: value["action"]}
	if len(value["user"]) == 0 {
		err = fmt.Errorf("Slug not found.")
	}
	return mappedValue, err
}

func createRedisEntry(input RedisInput, client *redis.Client) (string, error) {
	slug := ulid.Make().String()
	timeDuration := time.Duration(input.Ttl) * time.Second
	err := client.HSet(ctx, slug, map[string]interface{}{"user": input.User, "userType": input.UserType, "action": input.Action}).Err()

	if err == nil {
		err = client.ExpireLT(ctx, slug, timeDuration).Err()
		log.Info("Created Redis entry with slug %s expiring in %s", slug, timeDuration)
	}
	return slug, err
}

func main() {
	client := getRedisClient()

	http.HandleFunc("GET /lookup/{slug}", func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("Content-Type", "application/json")
		slug := request.PathValue("slug")
		redisOutput, err := getRedisEntryBySlug(slug, client)
		if err != nil {
			errorResponse := ErrorResponse{StatusCode: 404, ErrorMessage: "Key not found"}
			log.Warn(errorResponse)
			json.NewEncoder(responseWriter).Encode(errorResponse)
		} else {
			log.Println(redisOutput)
			json.NewEncoder(responseWriter).Encode(redisOutput)
		}
	})
	http.HandleFunc("GET /save/{userType}/{userId}/{action}/{ttl}", func(responseWriter http.ResponseWriter, request *http.Request) {
		// The default response, unless overridden by success
		response := "Error creating slug."
		// Sanitize input parameters
		pathUserType, pathAction, pathUserId, ttl, err := sanitizeInputParams(request)
		if err != nil {
			log.Println(err)
		}
		// Create a Redis entry
		slug, err := createRedisEntry(RedisInput{
			User:     pathUserId,
			UserType: pathUserType,
			Action:   pathAction,
			Ttl:      ttl,
		}, client)
		if err == nil {
			// On success, replace the default response
			response = slug
		} else {
			log.Println(err)
		}
		// Always write the response (i.e. return)
		fmt.Fprintf(responseWriter, response)
	})
	http.HandleFunc("GET /save/{userType}/{userId}/{action}", func(responseWriter http.ResponseWriter, request *http.Request) {
		// The default response, unless overridden by success
		response := "Error creating slug."
		// Sanitize input parameters
		pathUserType, pathAction, pathUserId, ttl, err := sanitizeInputParams(request)
		if err != nil {
			log.Println(err)
		}
		// Create a Redis entry
		slug, err := createRedisEntry(RedisInput{
			User:     pathUserId,
			UserType: pathUserType,
			Action:   pathAction,
			Ttl:      ttl,
		}, client)
		if err == nil {
			// On success, replace the default response
			response = slug
		} else {
			log.Println(err)
		}
		// Always write the response (i.e. return)
		fmt.Fprintf(responseWriter, response)
	})
	log.Fatal(http.ListenAndServe(":8080", nil))
}
func sanitizeInputParams(request *http.Request) (string, string, string, int, error) {
	userType := request.PathValue("userType")
	if userType != "prospect" && userType != "resident" {
		return "", "", "", 0, fmt.Errorf("Invalid user type.")
	}
	action := request.PathValue("action")
	if action != "smsConsent" && action != "appointmentChange" {
		return "", "", "", 0, fmt.Errorf("Invalid action type.")
	}
	// Don't validate userId yet. We don't know the constraints.
	userId := request.PathValue("userId")
	ttl := parseTTL(request.PathValue("ttl"))
	return userType, action, userId, ttl, nil
}
func parseTTL(userTtl string) int {
	ttl := DEFAULT_TTL
	intTtl, err := strconv.Atoi(userTtl)
	if err == nil && intTtl > 0 && intTtl <= TTL_MAX {
		ttl = intTtl
	}
	return ttl
}
