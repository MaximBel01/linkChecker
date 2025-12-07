# Test the Link Checker API

Write-Host "=== Testing Link Checker API ===" -ForegroundColor Green
Write-Host ""

# Test 1: Health Check
Write-Host "Test 1: Health Check" -ForegroundColor Cyan
try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/health" -Method Get
    Write-Host "Status Code: $($response.StatusCode)"
    Write-Host "Response: $($response.Content)" -ForegroundColor Green
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}

Write-Host ""

# Test 2: Submit links for checking
Write-Host "Test 2: Submit links for checking" -ForegroundColor Cyan
$json = @{
    links = @(
        "https://www.google.com",
        "https://www.github.com",
        "https://invalid-domain-12345.com",
        "https://www.stackoverflow.com"
    )
} | ConvertTo-Json

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/check" -Method Post -ContentType "application/json" -Body $json
    Write-Host "Status Code: $($response.StatusCode)"
    $responseData = $response.Content | ConvertFrom-Json
    Write-Host "Response:" -ForegroundColor Green
    Write-Host ($responseData | ConvertTo-Json -Depth 10)
    $batchId = $responseData.batch_id
} catch {
    Write-Host "Error: $_" -ForegroundColor Red
}

Write-Host ""

# Test 3: Wait a bit for results
Write-Host "Test 3: Waiting for link checks to complete..." -ForegroundColor Cyan
Start-Sleep -Seconds 5

# Test 4: Get PDF report
if ($batchId) {
    Write-Host "Test 4: Generate PDF report for batch $batchId" -ForegroundColor Cyan
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:8080/report?batch_ids=$batchId" -Method Get -OutFile "report_$batchId.pdf"
        Write-Host "Status Code: $($response.StatusCode)"
        Write-Host "PDF report saved to: report_$batchId.pdf" -ForegroundColor Green
    } catch {
        Write-Host "Error: $_" -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "=== Tests Complete ===" -ForegroundColor Green
