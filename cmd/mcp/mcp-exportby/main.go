package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/xuri/excelize/v2"
	_ "modernc.org/sqlite"
)

// --- API types ---

type apiCompany struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Logo        string `json:"logo"`
	Description string `json:"description"`
	Country     string `json:"country"`
	IsFavorite  bool   `json:"is_favorite"`
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

	s := server.NewMCPServer("mcp-exportby", "1.0.0")

	s.AddTool(mcp.NewTool("sync_catalog",
		mcp.WithDescription("Скачивает компании из каталога export.by и сохраняет в локальную БД. Продолжает с последней просканированной страницы. Возвращает статистику."),
		mcp.WithNumber("max_pages", mcp.Description("Максимальное количество страниц для скачивания за один вызов (по умолчанию 50, макс 500).")),
		mcp.WithNumber("from_page", mcp.Description("Начальная страница (по умолчанию — продолжение с последней просканированной).")),
		mcp.WithString("country", mcp.Description("Код страны для фильтрации (по умолчанию BY).")),
	), handleSyncCatalog)

	s.AddTool(mcp.NewTool("get_unanalyzed",
		mcp.WithDescription("Возвращает компании из raw_companies, которых ещё нет в таблице companies (не проанализированы). Для передачи агенту-аналитику."),
		mcp.WithNumber("limit", mcp.Description("Максимальное количество компаний (по умолчанию 100).")),
		mcp.WithNumber("offset", mcp.Description("Смещение для пагинации (по умолчанию 0).")),
	), handleGetUnanalyzed)

	s.AddTool(mcp.NewTool("check_new",
		mcp.WithDescription("Проверяет первые N страниц каталога на наличие новых компаний. Возвращает количество новых."),
		mcp.WithNumber("pages", mcp.Description("Количество первых страниц для проверки (по умолчанию 3).")),
	), handleCheckNew)

	s.AddTool(mcp.NewTool("get_stats",
		mcp.WithDescription("Возвращает статистику по локальной БД: количество компаний, прогресс сканирования, дата последнего обновления."),
	), handleGetStats)

	s.AddTool(mcp.NewTool("mark_exported",
		mcp.WithDescription("Помечает все лиды со статусом 'new' как 'sent' в таблице companies и записывает sent_at. Вызывай после успешной отправки отчёта."),
	), handleMarkExported)

	s.AddTool(mcp.NewTool("reject_companies",
		mcp.WithDescription("Помечает компании как отклонённые (импортёры, сервисные и т.д.). Они больше не будут появляться в get_unanalyzed. Вызывай для КАЖДОЙ отклонённой компании после анализа."),
		mcp.WithArray("names", mcp.Required(), mcp.Description("Массив названий компаний для отклонения."), mcp.WithStringItems()),
		mcp.WithString("reason", mcp.Description("Причина отклонения (importer, service, distributor и т.д.).")),
	), handleRejectCompanies)

	s.AddTool(mcp.NewTool("get_pending_count",
		mcp.WithDescription("Возвращает количество лидов со статусом 'new' в таблице companies (готовых к отправке)."),
	), handleGetPendingCount)

	s.AddTool(mcp.NewTool("export_leads_excel",
		mcp.WithDescription("Генерирует Excel-файл из всех лидов со статусом 'new'. Возвращает путь к файлу и количество лидов. Вызывай перед отправкой в Telegram/email."),
	), handleExportLeadsExcel)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
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
		CREATE TABLE IF NOT EXISTS rejected_companies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			reason TEXT,
			rejected_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_raw_companies_export_by_id ON raw_companies(export_by_id);
		CREATE INDEX IF NOT EXISTS idx_raw_companies_products_fetched ON raw_companies(products_fetched);
	`)
	if err != nil {
		return err
	}

	// Migration: add sent_at column to companies table (ignore error if already exists)
	db.Exec(`ALTER TABLE companies ADD COLUMN sent_at TEXT`)

	// Migration: add export_by_id column to companies table for deduplication
	db.Exec(`ALTER TABLE companies ADD COLUMN export_by_id INTEGER`)

	return nil
}

// --- sync_catalog ---

func handleSyncCatalog(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	maxPages := req.GetInt("max_pages", 50)
	fromPage := req.GetInt("from_page", 0)
	country := req.GetString("country", "BY")

	if maxPages <= 0 {
		maxPages = 50
	}
	if maxPages > maxPerCall {
		maxPages = maxPerCall
	}

	// Determine start page from scan_progress if not specified
	if fromPage <= 0 {
		var lastPage sql.NullInt64
		db.QueryRow("SELECT MAX(last_page) FROM scan_progress").Scan(&lastPage)
		if lastPage.Valid {
			fromPage = int(lastPage.Int64) + 1
		} else {
			fromPage = 1
		}
	}

	totalAdded := 0
	totalSkipped := 0
	lastPage := fromPage
	var totalPages, totalCompanies int

	for page := fromPage; page < fromPage+maxPages; page++ {
		companies, tp, tc, err := fetchCatalogPage(country, page)
		if err != nil {
			// Log partial progress
			if totalAdded > 0 || page > fromPage {
				saveScanProgress(lastPage, totalPages, totalCompanies, totalAdded)
			}
			return mcp.NewToolResultError(fmt.Sprintf("error fetching page %d: %v (processed %d pages, added %d)", page, err, page-fromPage, totalAdded)), nil
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
		"pages_scanned": lastPage - fromPage + 1,
		"from_page":     fromPage,
		"to_page":       lastPage,
		"total_pages":   totalPages,
		"total_on_site": totalCompanies,
		"new_added":     totalAdded,
		"duplicates":    totalSkipped,
		"scan_complete": lastPage >= totalPages,
	}

	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
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

// importerKeywords — ключевые слова в description, означающие импортёра/дистрибьютора/сервисную компанию.
// Такие компании автоматически отклоняются без участия LLM.
var importerKeywords = []string{
	"дистрибьютор", "дистрибутор", "дилер", "импортёр", "импортер",
	"официальный представитель", "официальный дилер",
	"импорт и продажа", "представительство",
	"салон красоты", "парикмахерская", "барбершоп",
	"ресторан", "кафе ", "бар ",
	"автосервис", "автомойка", "шиномонтаж",
	"ремонт телефонов", "ремонт техники",
	"репетитор", "языковые курсы", "курсы обучения", "центр обучения", "школа обучения", "услуги обучения",
}

func containsImporterKeyword(desc string) bool {
	lower := strings.ToLower(desc)
	for _, kw := range importerKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func handleGetUnanalyzed(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := req.GetInt("limit", 100)
	offset := req.GetInt("offset", 0)

	if limit <= 0 {
		limit = 100
	}

	// Fetch more than needed to compensate for auto-rejected importers
	fetchLimit := limit * 3

	rows, err := db.Query(`
		SELECT MIN(r.export_by_id) AS export_by_id, r.name, r.description, r.country
		FROM raw_companies r
		LEFT JOIN companies c ON r.name = c.name OR r.export_by_id = c.export_by_id
		LEFT JOIN rejected_companies rej ON r.name = rej.name
		WHERE c.id IS NULL AND rej.id IS NULL
		GROUP BY r.name
		ORDER BY MIN(r.id)
		LIMIT ? OFFSET ?
	`, fetchLimit, offset)
	if err != nil {
		return mcp.NewToolResultError("query error: " + err.Error()), nil
	}
	defer rows.Close()

	var companies []map[string]any
	var autoRejected []string
	autoRejectedReasons := make(map[string]string)
	for rows.Next() {
		var exportByID int
		var name, country string
		var description sql.NullString
		rows.Scan(&exportByID, &name, &description, &country)

		desc := ""
		if description.Valid {
			desc = description.String
		}

		// Auto-reject non-Belarusian companies
		if country != "" && country != "BY" && country != "Беларусь" {
			autoRejected = append(autoRejected, name)
			autoRejectedReasons[name] = "auto:non_by_country"
			continue
		}

		// Auto-reject obvious importers/service companies
		if containsImporterKeyword(desc) {
			autoRejected = append(autoRejected, name)
			autoRejectedReasons[name] = "auto:importer_keyword"
			continue
		}

		if len(companies) < limit {
			companies = append(companies, map[string]any{
				"export_by_id": exportByID,
				"name":         name,
				"description":  nullStrVal(description),
				"country":      country,
				"url":          fmt.Sprintf("https://export.by/company/%d", exportByID),
			})
		}
	}

	// Persist auto-rejected companies so they don't appear again
	if len(autoRejected) > 0 {
		now := time.Now().Format("2006-01-02 15:04:05")
		for _, name := range autoRejected {
			reason := autoRejectedReasons[name]
			if reason == "" {
				reason = "auto:importer_keyword"
			}
			db.Exec(`INSERT OR IGNORE INTO rejected_companies (name, reason, rejected_at) VALUES (?, ?, ?)`, name, reason, now)
		}
	}

	// Total unanalyzed count (excluding both companies and rejected)
	var totalUnanalyzed int
	db.QueryRow(`
		SELECT COUNT(DISTINCT r.name) FROM raw_companies r
		LEFT JOIN companies c ON r.name = c.name OR r.export_by_id = c.export_by_id
		LEFT JOIN rejected_companies rej ON r.name = rej.name
		WHERE c.id IS NULL AND rej.id IS NULL
	`).Scan(&totalUnanalyzed)

	result := map[string]any{
		"companies":        companies,
		"returned":         len(companies),
		"total_unanalyzed": totalUnanalyzed,
		"auto_rejected":    len(autoRejected),
	}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

// --- check_new ---

func handleCheckNew(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pages := req.GetInt("pages", 3)

	if pages <= 0 {
		pages = 3
	}

	totalNew := 0
	var newCompanies []map[string]string

	for page := 1; page <= pages; page++ {
		companies, _, _, err := fetchCatalogPage("BY", page)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("error fetching page %d: %v", page, err)), nil
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
		"pages_checked": pages,
		"new_companies": totalNew,
		"new_list":      newCompanies,
	}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

// --- get_stats ---

func handleGetStats(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	return mcp.NewToolResultText(string(data)), nil
}

// --- mark_exported ---

func handleMarkExported(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	res, err := db.Exec(`UPDATE companies SET status = 'sent', sent_at = ? WHERE status = 'new'`, now)
	if err != nil {
		return mcp.NewToolResultError("update error: " + err.Error()), nil
	}
	affected, _ := res.RowsAffected()
	result := map[string]any{
		"updated": affected,
		"sent_at": now,
	}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

// --- reject_companies ---

func handleRejectCompanies(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var a struct {
		Names  []string `json:"names"`
		Reason string   `json:"reason"`
	}
	if err := req.BindArguments(&a); err != nil {
		return mcp.NewToolResultError("invalid params: " + err.Error()), nil
	}
	if len(a.Names) == 0 {
		return mcp.NewToolResultError("names array is required"), nil
	}
	if a.Reason == "" {
		a.Reason = "manual"
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	rejected := 0
	for _, name := range a.Names {
		_, err := db.Exec(`INSERT OR IGNORE INTO rejected_companies (name, reason, rejected_at) VALUES (?, ?, ?)`, name, a.Reason, now)
		if err == nil {
			rejected++
		}
	}

	result := map[string]any{"rejected": rejected}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

// --- get_pending_count ---

func handleGetPendingCount(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM companies WHERE status = 'new'`).Scan(&count)
	if err != nil {
		return mcp.NewToolResultError("query error: " + err.Error()), nil
	}
	result := map[string]any{
		"pending_count": count,
	}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

// --- export_leads_excel ---

func handleExportLeadsExcel(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dataDir := os.Getenv("DOMAIN_DATA_DIR")
	if dataDir == "" {
		dataDir = "."
	}

	rows, err := db.Query(`
		SELECT name, url, description, products, contact_info, export_destinations,
		       aviation_priority, found_date
		FROM companies WHERE status = 'new'
		ORDER BY aviation_priority DESC, found_date DESC
	`)
	if err != nil {
		return mcp.NewToolResultError("query error: " + err.Error()), nil
	}
	defer rows.Close()

	type lead struct {
		Name, URL, Description, Products, ContactInfo, ExportDest string
		Priority                                                  int
		FoundDate                                                 string
	}
	var leads []lead
	for rows.Next() {
		var l lead
		var url, desc, products, contact, export sql.NullString
		if err := rows.Scan(&l.Name, &url, &desc, &products, &contact, &export, &l.Priority, &l.FoundDate); err != nil {
			continue
		}
		l.URL = nullStr(url)
		l.Description = nullStr(desc)
		l.Products = nullStr(products)
		l.ContactInfo = nullStr(contact)
		l.ExportDest = nullStr(export)
		leads = append(leads, l)
	}

	if len(leads) == 0 {
		return mcp.NewToolResultError("no pending leads to export"), nil
	}

	f := excelize.NewFile()
	sheet := "Лиды"
	f.SetSheetName("Sheet1", sheet)

	// Headers
	headers := []string{"Приоритет", "Компания", "Описание", "Продукция", "Экспорт", "Контакты", "URL", "Дата"}
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 11},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"2F5496"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "bottom", Color: "000000", Style: 2},
		},
	})
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
	}

	// Priority labels
	priorityLabel := map[int]string{2: "🔴 Высокий", 1: "🟡 Средний", 0: "🟢 Низкий"}

	// Data rows
	for i, l := range leads {
		row := i + 2
		pLabel := priorityLabel[l.Priority]
		if pLabel == "" {
			pLabel = fmt.Sprintf("%d", l.Priority)
		}
		vals := []any{pLabel, l.Name, l.Description, l.Products, l.ExportDest, l.ContactInfo, l.URL, l.FoundDate}
		for j, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(j+1, row)
			f.SetCellValue(sheet, cell, v)
		}
	}

	// Column widths
	widths := map[string]float64{"A": 16, "B": 30, "C": 40, "D": 30, "E": 20, "F": 25, "G": 35, "H": 12}
	for col, w := range widths {
		f.SetColWidth(sheet, col, col, w)
	}

	// Count by priority
	var high, medium int
	for _, l := range leads {
		switch l.Priority {
		case 2:
			high++
		case 1:
			medium++
		}
	}

	filename := fmt.Sprintf("leads_%s.xlsx", time.Now().Format("2006-01-02_150405"))
	outPath := filepath.Join(dataDir, filename)
	if err := f.SaveAs(outPath); err != nil {
		return mcp.NewToolResultError("save excel error: " + err.Error()), nil
	}

	result := map[string]any{
		"file_path":     outPath,
		"total_leads":   len(leads),
		"high_priority": high,
		"med_priority":  medium,
	}
	data, _ := json.Marshal(result)
	return mcp.NewToolResultText(string(data)), nil
}

func nullStr(s sql.NullString) string {
	if s.Valid {
		return s.String
	}
	return ""
}

// --- helpers ---

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
