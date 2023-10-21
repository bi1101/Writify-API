package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

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
}

func handleRequest(promptFile string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		token := r.Header.Get("TOKEN")

		if strings.TrimSpace(token) == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if r.ContentLength == 0 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(w, "Error %v", r)
			}
		}()

		defer r.Body.Close()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}

		var req AskRequest
		if err := json.Unmarshal(data, &req); err != nil {
			panic(err)
		}

		switch req.Stream {
		case true:
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			if !ok {
				panic(fmt.Errorf("webserver doesn't support hijacking!"))
			}

			for it := range askWithStream(promptFile, token, req) {
				if it.err != nil {
					panic(err)
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
					panic(err)
				}
				fmt.Fprintf(w, "data: %v\n\n", string(data))
				flusher.Flush()
			}
		case false:
			w.Header().Set("Content-Type", "application/json")
			response, err := askWithoutStream(promptFile, token, req)
			if err != nil {
				panic(err)
			}
			if err := json.NewEncoder(w).Encode(response); err != nil {
				panic(err)
			}
		}
	}
}

func logMiddleware(next http.HandlerFunc) http.HandlerFunc {
	logger := log.New(os.Stdout, " [ESSAY_QUESTION_API] ", log.Ldate|log.Ltime)
	return func(w http.ResponseWriter, r *http.Request) {
		rl := NewCustomWriter(w)
		next.ServeHTTP(rl, r)
		logger.Printf("Status: [%v] Method: [%v] Path: [%v]", rl.status, r.Method, r.URL.Path)
	}
}

func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept, TOKEN")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	}
}

type CustomWriter struct {
	status int

	http.ResponseWriter
}

func NewCustomWriter(w http.ResponseWriter) *CustomWriter {
	return &CustomWriter{
		status:         200,
		ResponseWriter: w,
	}
}

func (c CustomWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := c.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	return h.Hijack()
}

func (c CustomWriter) Flush() {
	h, ok := c.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	h.Flush()
}

func (c *CustomWriter) WriteHeader(statusCode int) {
	c.status = statusCode
	c.ResponseWriter.WriteHeader(statusCode)
}

func main() {

	http.HandleFunc("/ask", logMiddleware(corsMiddleware(handleRequest("prompts/ask_prompt.txt"))))
	http.HandleFunc("/vocabulary-upgrade", logMiddleware(corsMiddleware(handleRequest("prompts/vocabulary-upgrade-prompt.txt"))))
	http.HandleFunc("/grammar-correction", logMiddleware(corsMiddleware(handleRequest("prompts/grammar-correction-prompt.txt"))))
	http.HandleFunc("/improved-task-2", logMiddleware(corsMiddleware(handleRequest("prompts/improved-task-2-prompt.txt"))))
	http.HandleFunc("/improved-task-1", logMiddleware(corsMiddleware(handleRequest("prompts/improved-task-1-prompt.txt"))))
	http.HandleFunc("/task-response", logMiddleware(corsMiddleware(handleRequest("prompts/task-response-prompt.txt"))))
	http.HandleFunc("/task-achievement", logMiddleware(corsMiddleware(handleRequest("prompts/task-achievement-prompt.txt"))))
	http.HandleFunc("/coherence-cohesion", logMiddleware(corsMiddleware(handleRequest("prompts/coherence-cohesion-prompt.txt"))))
	http.HandleFunc("/lexical-resource", logMiddleware(corsMiddleware(handleRequest("prompts/lexical-resource-prompt.txt"))))
	http.HandleFunc("/grammatical-range-accuracy", logMiddleware(corsMiddleware(handleRequest("prompts/grammatical-range-accuracy-prompt.txt"))))
	http.HandleFunc("/essay-outline", logMiddleware(corsMiddleware(handleRequest("prompts/essay-outline-prompt.txt"))))
	http.HandleFunc("/topic-vocabulary", logMiddleware(corsMiddleware(handleRequest("prompts/topic-vocabulary-prompt.txt"))))
	http.HandleFunc("/topic-analysis", logMiddleware(corsMiddleware(handleRequest("prompts/topic-analysis-prompt.txt"))))

	log.Printf("listening on :%v", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%v", port), nil); err != nil {
		log.Fatal(err)
	}
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
