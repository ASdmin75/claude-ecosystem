package main

import (
	"bufio"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

// --- JSON-RPC types ---

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// --- API types ---

type apiCompany struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Logo        string `json:"logo"`
	Description string `json:"description"`
	Country     string `json:"country"`
	IsFavorite  bool   `json:"is_favorite"`
}


// --- Tools ---

var tools = []tool{
	{
		Name:        "sync_catalog",
		Description: "Скачивает компании из каталога export.by и сохраняет в локальную БД. Продолжает с последней просканированной страницы. Возвращает статистику.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"max_pages": map[string]any{
					"type":        "integer",
					"description": "Максимальное количество страниц для скачивания за один вызов (по умолчанию 50, макс 500).",
				},
				"from_page": map[string]any{
					"type":        "integer",
					"description": "Начальная страница (по умолчанию — продолжение с последней просканированной).",
				},
				"country": map[string]any{
					"type":        "string",
					"description": "Код страны для фильтрации (по умолчанию BY).",
				},
			},
		},
	},
	{
		Name:        "get_unanalyzed",
		Description: "Возвращает компании из raw_companies, которых ещё нет в таблице companies (не проанализированы). Для передачи агенту-аналитику.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "Максимальное количество компаний (по умолчанию 100).",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Смещение для пагинации (по умолчанию 0).",
				},
			},
		},
	},
	{
		Name:        "check_new",
		Description: "Проверяет первые N страниц каталога на наличие новых компаний. Возвращает количество новых.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pages": map[string]any{
					"type":        "integer",
					"description": "Количество первых страниц для проверки (по умолчанию 3).",
				},
			},
		},
	},
	{
		Name:        "get_stats",
		Description: "Возвращает статистику по локальной БД: количество компаний, прогресс сканирования, дата последнего обновления.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
}

const (
	baseURL    = "https://export.by/back"
	perPage    = 50
	maxPerCall = 500
)

var (
	db     *sql.DB
	client *http.Client
)

func main() {
	dbPath := os.Getenv("DOMAIN_DB_PATH")
	if dbPath == "" {
		fmt.Fprintf(os.Stderr, "DOMAIN_DB_PATH environment variable is required\n")
		os.Exit(1)
	}

	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	if err := ensureSchema(); err != nil {
		fmt.Fprintf(os.Stderr, "schema error: %v\n", err)
		os.Exit(1)
	}

	client = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		var req jsonRPCRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}

		var resp jsonRPCResponse
		resp.JSONRPC = "2.0"
		resp.ID = req.ID

		switch req.Method {
		case "initialize":
			resp.Result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":   map[string]any{"tools": map[string]any{}},
				"serverInfo":     map[string]any{"name": "mcp-exportby", "version": "1.0.0"},
			}
		case "tools/list":
			resp.Result = map[string]any{"tools": tools}
		case "tools/call":
			resp.Result = handleToolCall(req.Params)
		default:
			resp.Error = map[string]any{"code": -32601, "message": "method not found"}
		}

		enc.Encode(resp)
	}
}

func ensureSchema() error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS raw_companies (
			id INTEGER PRIMARY KEY,
			export_by_id INTEGER UNIQUE NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			country TEXT,
			logo TEXT,
			products TEXT,
			products_fetched INTEGER DEFAULT 0,
			scraped_at TEXT NOT NULL,
			updated_at TEXT
		);
		CREATE TABLE IF NOT EXISTS scan_progress (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			last_page INTEGER NOT NULL,
			total_pages INTEGER,
			total_companies INTEGER,
			companies_added INTEGER DEFAULT 0,
			scanned_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_raw_companies_export_by_id ON raw_companies(export_by_id);
		CREATE INDEX IF NOT EXISTS idx_raw_companies_products_fetched ON raw_companies(products_fetched);
	`)
	return err
}

func handleToolCall(params json.RawMessage) toolResult {
	var p toolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return errorResult("invalid params: " + err.Error())
	}

	switch p.Name {
	case "sync_catalog":
		return handleSyncCatalog(p.Arguments)
	case "get_unanalyzed":
		return handleGetUnanalyzed(p.Arguments)
	case "check_new":
		return handleCheckNew(p.Arguments)
	case "get_stats":
		return handleGetStats()
	default:
		return errorResult("unknown tool: " + p.Name)
	}
}

// --- sync_catalog ---

func handleSyncCatalog(args json.RawMessage) toolResult {
	var a struct {
		MaxPages int    `json:"max_pages"`
		FromPage int    `json:"from_page"`
		Country  string `json:"country"`
	}
	json.Unmarshal(args, &a)

	if a.MaxPages <= 0 {
		a.MaxPages = 50
	}
	if a.MaxPages > maxPerCall {
		a.MaxPages = maxPerCall
	}
	if a.Country == "" {
		a.Country = "BY"
	}

	// Determine start page from scan_progress if not specified
	if a.FromPage <= 0 {
		var lastPage sql.NullInt64
		db.QueryRow("SELECT MAX(last_page) FROM scan_progress").Scan(&lastPage)
		if lastPage.Valid {
			a.FromPage = int(lastPage.Int64) + 1
		} else {
			a.FromPage = 1
		}
	}

	totalAdded := 0
	totalSkipped := 0
	lastPage := a.FromPage
	var totalPages, totalCompanies int

	for page := a.FromPage; page < a.FromPage+a.MaxPages; page++ {
		companies, tp, tc, err := fetchCatalogPage(a.Country, page)
		if err != nil {
			// Log partial progress
			if totalAdded > 0 || page > a.FromPage {
				saveScanProgress(lastPage, totalPages, totalCompanies, totalAdded)
			}
			return errorResult(fmt.Sprintf("error fetching page %d: %v (processed %d pages, added %d)", page, err, page-a.FromPage, totalAdded))
		}

		totalPages = tp
		totalCompanies = tc

		if len(companies) == 0 || page > totalPages {
			break
		}

		added, skipped := upsertCompanies(companies)
		totalAdded += added
		totalSkipped += skipped
		lastPage = page

		// Small delay to be respectful
		time.Sleep(200 * time.Millisecond)
	}

	saveScanProgress(lastPage, totalPages, totalCompanies, totalAdded)

	result := map[string]any{
		"pages_scanned":   lastPage - a.FromPage + 1,
		"from_page":       a.FromPage,
		"to_page":         lastPage,
		"total_pages":     totalPages,
		"total_on_site":   totalCompanies,
		"new_added":       totalAdded,
		"duplicates":      totalSkipped,
		"scan_complete":   lastPage >= totalPages,
	}

	data, _ := json.Marshal(result)
	return textResult(string(data))
}

func fetchCatalogPage(country string, page int) ([]apiCompany, int, int, error) {
	url := fmt.Sprintf("%s/search/company?country=%s&page=%d&per-page=%d", baseURL, country, page, perPage)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		// Rate limited — wait and signal error
		return nil, 0, 0, fmt.Errorf("rate limited (429), try again later")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, 0, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(len(body), 200)]))
	}

	totalPages, _ := strconv.Atoi(resp.Header.Get("X-Pagination-Page-Count"))
	totalCount, _ := strconv.Atoi(resp.Header.Get("X-Pagination-Total-Count"))

	var companies []apiCompany
	if err := json.NewDecoder(resp.Body).Decode(&companies); err != nil {
		return nil, 0, 0, fmt.Errorf("JSON decode: %v", err)
	}

	return companies, totalPages, totalCount, nil
}

func upsertCompanies(companies []apiCompany) (added, skipped int) {
	now := time.Now().Format("2006-01-02 15:04:05")

	for _, c := range companies {
		res, err := db.Exec(`
			INSERT INTO raw_companies (export_by_id, name, description, country, logo, scraped_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(export_by_id) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				country = excluded.country,
				logo = excluded.logo,
				updated_at = ?
		`, c.ID, c.Name, c.Description, c.Country, c.Logo, now, now)
		if err != nil {
			skipped++
			continue
		}
		affected, _ := res.RowsAffected()
		if affected > 0 {
			// Check if this was a real insert vs update by checking if updated_at was set
			var updatedAt sql.NullString
			db.QueryRow("SELECT updated_at FROM raw_companies WHERE export_by_id = ?", c.ID).Scan(&updatedAt)
			if !updatedAt.Valid {
				added++
			} else {
				skipped++
			}
		}
	}
	return
}

func saveScanProgress(lastPage, totalPages, totalCompanies, companiesAdded int) {
	now := time.Now().Format("2006-01-02 15:04:05")
	db.Exec(`INSERT INTO scan_progress (last_page, total_pages, total_companies, companies_added, scanned_at)
		VALUES (?, ?, ?, ?, ?)`, lastPage, totalPages, totalCompanies, companiesAdded, now)
}

// --- get_unanalyzed ---

func handleGetUnanalyzed(args json.RawMessage) toolResult {
	var a struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
	}
	json.Unmarshal(args, &a)

	if a.Limit <= 0 {
		a.Limit = 100
	}

	rows, err := db.Query(`
		SELECT r.export_by_id, r.name, r.description, r.country
		FROM raw_companies r
		LEFT JOIN companies c ON r.name = c.name
		WHERE c.id IS NULL
		ORDER BY r.id
		LIMIT ? OFFSET ?
	`, a.Limit, a.Offset)
	if err != nil {
		return errorResult("query error: " + err.Error())
	}
	defer rows.Close()

	var companies []map[string]any
	for rows.Next() {
		var exportByID int
		var name, country string
		var description sql.NullString
		rows.Scan(&exportByID, &name, &description, &country)
		companies = append(companies, map[string]any{
			"export_by_id": exportByID,
			"name":         name,
			"description":  nullStrVal(sql.NullString{String: description.String, Valid: description.Valid}),
			"country":      country,
		})
	}

	// Also get total unanalyzed count
	var totalUnanalyzed int
	db.QueryRow(`
		SELECT COUNT(*) FROM raw_companies r
		LEFT JOIN companies c ON r.name = c.name
		WHERE c.id IS NULL
	`).Scan(&totalUnanalyzed)

	result := map[string]any{
		"companies":        companies,
		"returned":         len(companies),
		"total_unanalyzed": totalUnanalyzed,
	}
	data, _ := json.Marshal(result)
	return textResult(string(data))
}

// --- check_new ---

func handleCheckNew(args json.RawMessage) toolResult {
	var a struct {
		Pages int `json:"pages"`
	}
	json.Unmarshal(args, &a)

	if a.Pages <= 0 {
		a.Pages = 3
	}

	totalNew := 0
	var newCompanies []map[string]string

	for page := 1; page <= a.Pages; page++ {
		companies, _, _, err := fetchCatalogPage("BY", page)
		if err != nil {
			return errorResult(fmt.Sprintf("error fetching page %d: %v", page, err))
		}

		for _, c := range companies {
			var exists int
			err := db.QueryRow("SELECT 1 FROM raw_companies WHERE export_by_id = ?", c.ID).Scan(&exists)
			if err == sql.ErrNoRows {
				// New company — add it
				now := time.Now().Format("2006-01-02 15:04:05")
				db.Exec(`INSERT INTO raw_companies (export_by_id, name, description, country, logo, scraped_at)
					VALUES (?, ?, ?, ?, ?, ?)`, c.ID, c.Name, c.Description, c.Country, c.Logo, now)
				totalNew++
				newCompanies = append(newCompanies, map[string]string{
					"id":   strconv.Itoa(c.ID),
					"name": c.Name,
				})
			}
		}

		time.Sleep(200 * time.Millisecond)
	}

	result := map[string]any{
		"pages_checked":  a.Pages,
		"new_companies":  totalNew,
		"new_list":       newCompanies,
	}
	data, _ := json.Marshal(result)
	return textResult(string(data))
}

// --- get_stats ---

func handleGetStats() toolResult {
	var totalCompanies, withProducts, withoutProducts int
	db.QueryRow("SELECT COUNT(*) FROM raw_companies").Scan(&totalCompanies)
	db.QueryRow("SELECT COUNT(*) FROM raw_companies WHERE products_fetched = 1").Scan(&withProducts)
	db.QueryRow("SELECT COUNT(*) FROM raw_companies WHERE products_fetched = 0").Scan(&withoutProducts)

	var lastScanPage sql.NullInt64
	var lastScanDate sql.NullString
	var totalPages sql.NullInt64
	var totalOnSite sql.NullInt64
	db.QueryRow(`SELECT last_page, total_pages, total_companies, scanned_at
		FROM scan_progress ORDER BY id DESC LIMIT 1`).Scan(&lastScanPage, &totalPages, &totalOnSite, &lastScanDate)

	scanComplete := false
	if lastScanPage.Valid && totalPages.Valid && lastScanPage.Int64 >= totalPages.Int64 {
		scanComplete = true
	}

	result := map[string]any{
		"total_in_db":       totalCompanies,
		"with_products":     withProducts,
		"without_products":  withoutProducts,
		"last_scanned_page": nullIntVal(lastScanPage),
		"total_pages":       nullIntVal(totalPages),
		"total_on_site":     nullIntVal(totalOnSite),
		"last_scan_date":    nullStrVal(lastScanDate),
		"scan_complete":     scanComplete,
	}
	data, _ := json.Marshal(result)
	return textResult(string(data))
}

// --- helpers ---

func textResult(text string) toolResult {
	return toolResult{Content: []contentItem{{Type: "text", Text: text}}}
}

func errorResult(msg string) toolResult {
	return toolResult{Content: []contentItem{{Type: "text", Text: msg}}, IsError: true}
}

func nullIntVal(n sql.NullInt64) any {
	if n.Valid {
		return n.Int64
	}
	return nil
}

func nullStrVal(s sql.NullString) any {
	if s.Valid {
		return s.String
	}
	return nil
}
