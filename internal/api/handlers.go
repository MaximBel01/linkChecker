package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"linkChecker/internal/checker"
	"linkChecker/internal/pdf"
	"linkChecker/internal/storage"
)

type Handler struct {
	checker *checker.LinkChecker
	storage *storage.Storage
	pdf     *pdf.Generator
}

func NewHandler(checker *checker.LinkChecker, storage *storage.Storage, pdfGen *pdf.Generator) *Handler {
	return &Handler{
		checker: checker,
		storage: storage,
		pdf:     pdfGen,
	}
}

type CheckLinksRequest struct {
	Links []string `json:"links"`
}

type CheckLinksResponse struct {
	BatchID int64  `json:"batch_id"`
	Links   any    `json:"links"`
	Message string `json:"message"`
}

func (h *Handler) HandleCheckLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CheckLinksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if len(req.Links) == 0 {
		http.Error(w, "No links provided", http.StatusBadRequest)
		return
	}

	batchID := h.storage.SaveBatch(req.Links)

	h.storage.UpdateBatch(batchID, []storage.LinkResult{}, "processing")

	go func() {
		results := h.checker.CheckLinks(req.Links)

		// Convert checker results to storage LinkResult format
		var linkResults []storage.LinkResult
		for _, result := range results {
			linkResult := storage.LinkResult{
				URL:       result.URL,
				Status:    result.Status,
				Available: result.Available,
				CheckedAt: result.CheckedAt,
				Error:     result.Error,
			}
			linkResults = append(linkResults, linkResult)
		}

		h.storage.UpdateBatch(batchID, linkResults, "completed")
	}()

	w.Header().Set("Content-Type", "application/json")
	response := CheckLinksResponse{
		BatchID: batchID,
		Links:   req.Links,
		Message: "Links are being checked. Use batch_id to retrieve the report.",
	}
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) HandleGetReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	batchIDsStr := r.URL.Query().Get("batch_ids")
	if batchIDsStr == "" {
		http.Error(w, "batch_ids parameter is required", http.StatusBadRequest)
		return
	}

	batchIDsStrSlice := strings.Split(batchIDsStr, ",")
	var batchIDs []int64

	for _, idStr := range batchIDsStrSlice {
		id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid batch_id: %s", idStr), http.StatusBadRequest)
			return
		}
		batchIDs = append(batchIDs, id)
	}

	batches, err := h.storage.GetBatches(batchIDs)
	if err != nil || len(batches) == 0 {
		http.Error(w, "No batches found", http.StatusNotFound)
		return
	}

	pdfData, err := h.pdf.GenerateReport(batches)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate PDF: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"report_%d.pdf\"", time.Now().Unix()))
	w.Header().Set("Content-Length", strconv.Itoa(len(pdfData)))

	w.Write(pdfData)
}

type StatusResponse struct {
	BatchID int64    `json:"batch_id"`
	Status  string   `json:"status"`
	URLs    []string `json:"urls"`
	Results any      `json:"results,omitempty"`
}

func (h *Handler) HandleGetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	batchIDStr := r.URL.Query().Get("batch_id")
	if batchIDStr == "" {
		http.Error(w, "batch_id parameter is required", http.StatusBadRequest)
		return
	}

	batchID, err := strconv.ParseInt(batchIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid batch_id", http.StatusBadRequest)
		return
	}

	batch, err := h.storage.GetBatch(batchID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Batch not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	response := StatusResponse{
		BatchID: batch.BatchID,
		Status:  batch.Status,
		URLs:    batch.URLs,
	}

	if batch.Status == "completed" {
		response.Results = batch.Results
	}

	json.NewEncoder(w).Encode(response)
}

func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
