package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

type Prediction struct {
	CompletedAt string      `json:"completed_at"`
	CreatedAt   string      `json:"created_at"`
	Error       interface{} `json:"error"`
	ID          string      `json:"id"`
	Input       struct {
		Prompt   string  `json:"prompt"`
		Guidance float64 `json:"guidance"`
	} `json:"input"`
	Logs      string      `json:"logs"`
	Metrics   interface{} `json:"metrics"`
	Output    interface{} `json:"output"`
	StartedAt string      `json:"started_at"`
	Status    string      `json:"status"`
	Version   string      `json:"version"`
	URLs      struct {
		Get    string `json:"get"`
		Cancel string `json:"cancel"`
	} `json:"urls"`
}

var tmpl = template.Must(template.ParseGlob("templates/*.html"))

func main() {
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/generate", generateHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Println("Server gestartet auf :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl.ExecuteTemplate(w, "index.html", nil)
}

func generateHandler(w http.ResponseWriter, r *http.Request) {
	prompt := r.FormValue("prompt")

	apiKey := os.Getenv("REPLICATE_API_TOKEN")
	if apiKey == "" {
		http.Error(w, "Replicate API-Schlüssel nicht gesetzt", http.StatusInternalServerError)
		return
	}

	// Erstelle eine neue Prediction
	requestBody, _ := json.Marshal(map[string]interface{}{
		"input": map[string]interface{}{
			"prompt":   prompt,
			"guidance": 3.5,
		},
	})

	req, _ := http.NewRequest("POST", "https://api.replicate.com/v1/models/black-forest-labs/flux-dev/predictions", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Fehler bei der Bildgenerierung", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var prediction Prediction
	json.Unmarshal(body, &prediction)

	// Überprüfe auf Fehler
	if prediction.Error != nil {
		http.Error(w, "Fehler bei der Bildgenerierung: "+prediction.Error.(string), http.StatusInternalServerError)
		return
	}

	// Polling, bis die Prediction fertig ist
	for prediction.Status == "starting" || prediction.Status == "processing" {
		time.Sleep(1 * time.Second)

		req, _ := http.NewRequest("GET", prediction.URLs.Get, nil)
		req.Header.Set("Authorization", "Token "+apiKey)

		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "Fehler beim Abrufen des Bildes", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		json.Unmarshal(body, &prediction)

		if prediction.Error != nil {
			http.Error(w, "Fehler bei der Bildgenerierung: "+prediction.Error.(string), http.StatusInternalServerError)
			return
		}
	}

	if prediction.Status == "succeeded" {
		// Hol das Ausgabe-Bild
		output, ok := prediction.Output.([]interface{})
		if !ok || len(output) == 0 {
			http.Error(w, "Kein Bild generiert", http.StatusInternalServerError)
			return
		}

		imageURL, ok := output[0].(string)
		if !ok {
			http.Error(w, "Fehler beim Lesen des Bildes", http.StatusInternalServerError)
			return
		}

		tmpl.ExecuteTemplate(w, "result.html", imageURL)
	} else {
		http.Error(w, "Bildgenerierung fehlgeschlagen", http.StatusInternalServerError)
	}
}
