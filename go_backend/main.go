package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type PredictionResult struct {
	BatikType  string  `json:"batik_type"`
	Confidence float64 `json:"confidence"`
}

type Job struct {
	ID       string
	FilePath string
	ResultCh chan JobResult
}

type JobResult struct {
	Result PredictionResult
	Err    error
}

var JobQueue chan Job

func uploadHandler(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Access-Control-Allow-Headers, Authorization, X-Requested-With")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method tidak diperbolehkan", http.StatusMethodNotAllowed)
		return
	}

	file, handler, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "Gagal membaca gambar batik: ", http.StatusBadRequest)
		return
	}

	defer file.Close()

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	tempPath := filepath.Join("uploads", jobID+filepath.Ext(handler.Filename))
	dst, err := os.Create(tempPath)

	if err != nil {
		http.Error(w, "Gagal menyimpan file di server: ", http.StatusInternalServerError)
		return
	}

	_, err = io.Copy(dst, file)

	if err != nil {
		dst.Close()
		http.Error(w, "Gagal create file", http.StatusInternalServerError)
		return
	}

	dst.Close()

	//// defer dst.Close()

	//// io.Copy(dst, file)

	resultChannel := make(chan JobResult)

	JobQueue <- Job{
		ID:       jobID,
		FilePath: tempPath,
		ResultCh: resultChannel,
	}

	jobResult := <-resultChannel

	if jobResult.Err != nil {
		http.Error(w, "Gagal Memproses Gambar : "+jobResult.Err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobResult.Result)
}

func workerBatik(id int, jobs <-chan Job) {
	for job := range jobs {

		log.Printf("Worker membaca: %s \n", job.FilePath)

		fmt.Printf("Worker %d, Mulai memproses gambar ID: %s\n", id, job.ID)

		hasil, err := kirimHasilKePythonInference(job.FilePath)

		if err != nil {
			log.Printf("Gagal Kirim ke Python Inference : %v\n", err)

			job.ResultCh <- JobResult{
				Result: PredictionResult{},
				Err:    err,
			}
			continue
		}

		removeErr := os.Remove(job.FilePath)

		if removeErr != nil {
			log.Printf(
				"Gagal hapus file %s: %v",
				job.FilePath,
				removeErr,
			)
		} else {
			log.Printf(
				"Berhasil hapus file %s",
				job.FilePath,
			)
		}

		job.ResultCh <- JobResult{Result: hasil, Err: err}
		fmt.Printf("Worker %d, Selesai memproses gambar ID %s\n", id, job.ID)
	}
}

func kirimHasilKePythonInference(filePath string) (PredictionResult, error) {
	var hasil PredictionResult

	file, err := os.Open(filePath)
	if err != nil {
		return hasil, err
	}

	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", filepath.Base(filePath))

	if err != nil {
		return hasil, err
	}

	io.Copy(part, file)
	writer.Close()

	req, err := http.NewRequest("POST", "http://localhost:5005/predict", body)
	if err != nil {
		return hasil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)

	if err != nil {
		return hasil, fmt.Errorf("Python service offline: %v", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return hasil, fmt.Errorf("Respon dari python, status error: %d", resp.StatusCode)
	}

	err = json.NewDecoder(resp.Body).Decode(&hasil)

	if err != nil {
		return hasil, err
	}

	return hasil, nil
}

func cekStatusServer(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Access-Control-Allow-Headers, Authorization, X-Requested-With")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	client := http.Client{
		Timeout: 2 * time.Second,
	}

	goAlive := true

	// reqGo, err := client.Get("http://localhost:8080/status-go")

	// if err != nil {
	// 	goAlive = false
	// } else {
	// 	defer reqGo.Body.Close()

	// 	if reqGo.StatusCode != http.StatusOK {
	// 		goAlive = false
	// 	}
	// }

	modelAlive := true

	reqPython, err := client.Get("http://localhost:5005/status-python")

	if err != nil {
		modelAlive = false
	} else {
		defer reqPython.Body.Close()

		if reqPython.StatusCode != http.StatusOK {
			modelAlive = false
		}
	}

	json.NewEncoder(w).Encode(map[string]any{
		"status-service":      goAlive && modelAlive,
		"status-akhir-go":     goAlive,
		"status-akhir-python": modelAlive,
	})

}

func main() {
	// set hanya menampung 100 gambar.
	JobQueue = make(chan Job, 100)

	// maks worker saya set 3 saja yang jalan.
	for i := 1; i <= 3; i++ {
		go workerBatik(i, JobQueue)
	}

	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/status", cekStatusServer)

	fmt.Println("Go service berjalan di http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))

}
