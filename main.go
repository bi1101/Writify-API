package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
)

type Messages struct {
	Question string `json:"question"`
	Essay    string `json:"essay"`
}

type AskRequest struct {
	Model            string     `json:"model"`
	Messages         []Messages `json:"messages"`
	Temperature      float32    `json:"temperature"`
	TopP             float32    `json:"top_p"`
	N                int        `json:"n"`
	MaxTokens        int        `json:"max_tokens"`
	PresencePenalty  float32    `json:"presence_penalty"`
	FrequencyPenalty float32    `json:"frequency_penalty"`
	Stream           bool       `json:"stream"`
}

type AskResponse struct {
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices any    `json:"choices"`
}

var port string

func init() {
	godotenv.Load()
	port = os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
}

func handleRequest(promptFile string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			return
		}

		splitted := strings.Split(authHeader, " ")
		if len(splitted) != 2 || strings.ToLower(splitted[0]) != "bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header format must be 'Bearer [TOKEN]'"})
			return
		}

		token := splitted[1]

		var req AskRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		switch req.Stream {
		case true:
			c.Writer.Header().Set("Content-Type", "text/event-stream")
			c.Stream(func(w io.Writer) bool {
				for it := range askWithStream(promptFile, token, req) {
					if it.err != nil {
						c.String(http.StatusInternalServerError, "Error %v", it.err)
						return false
					}
					resp := AskResponse{
						Id:      it.Response.ID,
						Object:  it.Response.Object,
						Created: it.Response.Created,
						Model:   it.Response.Model,
						Choices: it.Response.Choices,
					}
					data, err := json.Marshal(resp)
					if err != nil {
						c.String(http.StatusInternalServerError, "Error %v", err)
						return false
					}
					fmt.Fprintf(w, "data: %v\n\n", string(data))
				}
				return false
			})
		case false:
			response, err := askWithoutStream(promptFile, token, req)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
				return
			}
			c.Writer.Header().Set("Content-Type", "application/json")
			c.JSON(http.StatusOK, response)
		}
	}
}

func corsMiddleware(c *gin.Context) {
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
	c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept, TOKEN")

	if c.Request.Method == http.MethodOptions {
		c.AbortWithStatus(http.StatusNoContent)
		return
	}
	c.Next()
}

func main() {
	r := gin.Default()

	r.Use(corsMiddleware)

	r.POST("/ask", handleRequest("prompts/ask_prompt.txt"))
	r.POST("/vocabulary-upgrade", handleRequest("prompts/vocabulary-upgrade-prompt.txt"))
	r.POST("/grammar-correction", handleRequest("prompts/grammar-correction-prompt.txt"))
	r.POST("/improved-task-2", handleRequest("prompts/improved-task-2-prompt.txt"))
	r.POST("/improved-task-1", handleRequest("prompts/improved-task-1-prompt.txt"))
	r.POST("/task-response", handleRequest("prompts/task-response-prompt.txt"))
	r.POST("/task-achievement", handleRequest("prompts/task-achievement-prompt.txt"))
	r.POST("/coherence-cohesion", handleRequest("prompts/coherence-cohesion-prompt.txt"))
	r.POST("/lexical-resource", handleRequest("prompts/lexical-resource-prompt.txt"))
	r.POST("/grammatical-range-accuracy", handleRequest("prompts/grammatical-range-accuracy-prompt.txt"))
	r.POST("/essay-outline", handleRequest("prompts/essay-outline-prompt.txt"))
	r.POST("/topic-vocabulary", handleRequest("prompts/topic-vocabulary-prompt.txt"))
	r.POST("/topic-analysis", handleRequest("prompts/topic-analysis-prompt.txt"))

	r.Run(fmt.Sprintf(":%v", port))
}

type AnswerStream struct {
	Response openai.ChatCompletionStreamResponse

	err error
}

func askWithoutStream(promptFile string, token string, req AskRequest) (openai.ChatCompletionResponse, error) {
	client := openai.NewClient(token)
	messages := []openai.ChatCompletionMessage{}
	data, err := os.ReadFile(promptFile)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}
	tmpl, err := template.New("openai").Parse(string(data))
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}
	buff := &bytes.Buffer{}
	for index := range req.Messages {
		if err := tmpl.Execute(buff, req.Messages[index]); err != nil {
			return openai.ChatCompletionResponse{}, err
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: buff.String(),
		})
	}
	return client.CreateChatCompletion(
		context.TODO(),
		openai.ChatCompletionRequest{
			Model:            req.Model,
			MaxTokens:        req.MaxTokens,
			Temperature:      req.Temperature,
			TopP:             req.TopP,
			N:                req.N,
			PresencePenalty:  req.PresencePenalty,
			FrequencyPenalty: req.FrequencyPenalty,
			Messages:         messages,
		},
	)
}

func askWithStream(promptFile string, token string, req AskRequest) <-chan AnswerStream {
	out := make(chan AnswerStream)

	go func() {
		defer close(out)
		client := openai.NewClient(token)
		data, err := os.ReadFile(promptFile)
		if err != nil {
			out <- AnswerStream{
				err: err,
			}
			return
		}

		messages := []openai.ChatCompletionMessage{}
		tmpl, err := template.New("openai").Parse(string(data))
		if err != nil {
			out <- AnswerStream{
				err: err,
			}
			return
		}
		buff := &bytes.Buffer{}
		for index := range req.Messages {
			if err := tmpl.Execute(buff, req.Messages[index]); err != nil {
				out <- AnswerStream{
					err: err,
				}
				return
			}
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: buff.String(),
			})
		}

		stream, err := client.CreateChatCompletionStream(
			context.TODO(),
			openai.ChatCompletionRequest{
				Model:            req.Model,
				MaxTokens:        req.MaxTokens,
				Temperature:      req.Temperature,
				TopP:             req.TopP,
				N:                req.N,
				PresencePenalty:  req.PresencePenalty,
				FrequencyPenalty: req.FrequencyPenalty,
				Messages:         messages,
				Stream:           true,
			},
		)
		if err != nil {
			out <- AnswerStream{
				err: err,
			}
			return
		}
		defer stream.Close()

		for {
			response, err := stream.Recv()
			if err != nil && err == io.EOF {
				return
			}
			if err != nil {
				out <- AnswerStream{
					err: err,
				}
				return
			}
			out <- AnswerStream{
				Response: response,
			}
		}
	}()

	return out
}
