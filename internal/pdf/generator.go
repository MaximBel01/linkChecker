package pdf

import (
	"fmt"
	"time"

	"linkChecker/internal/storage"

	"github.com/phpdave11/gofpdf"
)

type Generator struct{}

func NewGenerator() *Generator {
	return &Generator{}
}

func (g *Generator) GenerateReport(batches []*storage.LinkBatch) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")

	pdf.AddPage()

	pdf.SetFont("helvetica", "B", 20)
	pdf.Cell(200, 15, "Link Status Report")
	pdf.Ln(10)

	pdf.SetFont("helvetica", "", 10)
	pdf.Cell(200, 5, fmt.Sprintf("Generated: %s", getCurrentTime()))
	pdf.Ln(10)

	if len(batches) == 0 {
		pdf.SetFont("helvetica", "", 12)
		pdf.Cell(200, 10, "No batches found")
	} else {
		for _, batch := range batches {
			g.addBatchToReport(pdf, batch)
		}
	}

	var buf Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	return buf.Bytes(), nil
}

func (g *Generator) addBatchToReport(pdf *gofpdf.Fpdf, batch *storage.LinkBatch) {
	pdf.SetFont("helvetica", "B", 14)
	pdf.SetFillColor(200, 220, 255)
	pdf.CellFormat(190, 8, fmt.Sprintf("Batch #%d", batch.BatchID), "1", 1, "L", true, 0, "")
	pdf.Ln(2)

	pdf.SetFont("helvetica", "", 9)
	pdf.Cell(200, 5, fmt.Sprintf("Created: %s", batch.CreatedAt))
	pdf.Ln(5)
	pdf.Cell(200, 5, fmt.Sprintf("Status: %s", batch.Status))
	pdf.Ln(5)

	if len(batch.Results) > 0 {
		pdf.SetFont("helvetica", "B", 9)
		pdf.SetFillColor(220, 220, 220)

		colW := []float64{60, 20, 30, 45}
		pdf.CellFormat(colW[0], 7, "URL", "1", 0, "L", true, 0, "")
		pdf.CellFormat(colW[1], 7, "Status", "1", 0, "C", true, 0, "")
		pdf.CellFormat(colW[2], 7, "Available", "1", 0, "C", true, 0, "")
		pdf.CellFormat(colW[3], 7, "Checked At", "1", 1, "L", true, 0, "")

		pdf.SetFont("helvetica", "", 8)
		pdf.SetFillColor(245, 245, 245)

		for i, result := range batch.Results {
			fill := i%2 == 0
			if fill {
				pdf.SetFillColor(245, 245, 245)
			} else {
				pdf.SetFillColor(255, 255, 255)
			}

			url := result.URL
			if len(url) > 35 {
				url = url[:32] + "..."
			}

			status := fmt.Sprintf("%d", result.Status)
			available := fmt.Sprintf("%t", result.Available)
			checkedAt := result.CheckedAt
			if len(checkedAt) > 12 {
				checkedAt = checkedAt[:10]
			}

			pdf.CellFormat(colW[0], 6, url, "1", 0, "L", fill, 0, "")
			pdf.CellFormat(colW[1], 6, status, "1", 0, "C", fill, 0, "")
			pdf.CellFormat(colW[2], 6, available, "1", 0, "C", fill, 0, "")
			pdf.CellFormat(colW[3], 6, checkedAt, "1", 1, "L", fill, 0, "")
		}

		pdf.Ln(3)
	}

	if pdf.GetY() > 250 {
		pdf.AddPage()
	}
}

type Buffer struct {
	data []byte
}

func (b *Buffer) Write(p []byte) (n int, err error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *Buffer) Bytes() []byte {
	return b.data
}

func getCurrentTime() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func ConvertResultsForPDF(results any) []map[string]any {
	var converted []map[string]any

	if resultSlice, ok := results.([]storage.LinkResult); ok {
		for _, r := range resultSlice {
			resultMap := map[string]any{
				"url":        r.URL,
				"status":     r.Status,
				"available":  r.Available,
				"checked_at": r.CheckedAt,
			}
			if r.Error != "" {
				resultMap["error"] = r.Error
			}
			converted = append(converted, resultMap)
		}
	}

	return converted
}
