package main

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"pemira-backend/internal/config"
	"pemira-backend/internal/database"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/repository"
	"pemira-backend/internal/service"
	"pemira-backend/pkg/redis"
	"strings"
	"time"
)

// =============================================================================
// JSON-RPC 2.0 & MODEL CONTEXT PROTOCOL (MCP) STRUCTURES
// =============================================================================

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      interface{}   `json:"id"`
	Result  interface{}   `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type MCPToolResult struct {
	Content []MCPContent `json:"content"`
	IsError bool         `json:"isError,omitempty"`
}

type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ReadResourceParams struct {
	URI string `json:"uri"`
}

// =============================================================================
// MCP ACCESS CONTROL
// =============================================================================

// MCPAccessControl is the operator identity attached to a local MCP process.
// STDIO has no network authentication, so the launcher must choose the role and
// explicitly opt in to privileged tools via environment variables.
type MCPAccessControl struct {
	OperatorRole        string
	AllowVoteSubmit     bool
	AllowTally          bool
	AllowConfigMutation bool
	AllowAuditLogs      bool
	AllowLookupVoter    bool
	AllowMockLogin      bool
}

func accessControlFromConfig(cfg *config.Config) MCPAccessControl {
	return MCPAccessControl{
		OperatorRole:        cfg.MCPOperatorRole,
		AllowVoteSubmit:     cfg.MCPAllowVoteSubmit,
		AllowTally:          cfg.MCPAllowTally,
		AllowConfigMutation: cfg.MCPAllowConfigMutation,
		AllowAuditLogs:      cfg.MCPAllowAuditLogs,
		AllowLookupVoter:    cfg.MCPAllowLookupVoter,
		AllowMockLogin:      cfg.MCPAllowMockLogin,
	}
}

func (a MCPAccessControl) hasRole(roles ...string) bool {
	for _, role := range roles {
		if a.OperatorRole == role {
			return true
		}
	}
	return false
}

// =============================================================================
// PEMIRA 2026 MCP SERVER ENGINE
// =============================================================================

type PemiraMCPServer struct {
	cfg          *config.Config
	voterRepo    repository.VoterRepository
	ballotRepo   repository.BallotRepository
	chairRepo    repository.ChairRepository
	auditRepo    repository.AuditRepository
	authService  service.AuthService
	voteService  service.VoteService
	redisService *redis.RedisService
	acl          MCPAccessControl
	logger       *log.Logger

	in  io.Reader
	out io.Writer
}

func main() {
	// PENTING: Semua log runtime WAJIB diarahkan ke os.Stderr agar tidak merusak stream JSON-RPC di os.Stdout
	log.SetOutput(os.Stderr)
	logger := log.New(os.Stderr, "[PEMIRA-MCP] ", log.LstdFlags)

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatalf("Fatal: Konfigurasi tidak valid: %v", err)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		logger.Fatalf("Fatal: Gagal terhubung ke database: %v", err)
	}

	redisRequired := cfg.ServerEnv == "production" || cfg.RedisRequired
	redisService, err := redis.NewRedisService(cfg.RedisHost, cfg.RedisPort, cfg.RedisPassword, cfg.RedisDB, redisRequired)
	if err != nil {
		if redisRequired {
			logger.Fatalf("Fatal: Redis wajib aktif di production: %v", err)
		}
		logger.Printf("Warning: Koneksi Redis gagal (%v), menggunakan fallback database lock", err)
	}
	if redisService != nil {
		defer func() { _ = redisService.Close() }()
	}

	voterRepo := repository.NewVoterRepository(db)
	ballotRepo := repository.NewBallotRepository(db)
	chairRepo := repository.NewChairRepository(db)
	auditRepo := repository.NewAuditRepository(db)

	authService := service.NewAuthService(voterRepo, redisService, cfg)
	voteService := service.NewVoteService(db, voterRepo, ballotRepo, chairRepo, auditRepo, redisService, cfg)

	mcpServer := &PemiraMCPServer{
		cfg:          cfg,
		voterRepo:    voterRepo,
		ballotRepo:   ballotRepo,
		chairRepo:    chairRepo,
		auditRepo:    auditRepo,
		authService:  authService,
		voteService:  voteService,
		redisService: redisService,
		acl:          accessControlFromConfig(cfg),
		logger:       logger,
		in:           os.Stdin,
		out:          os.Stdout,
	}

	logger.Printf("PEMIRA 2026 MCP Server siap (role=%s, vote=%t, tally=%t, config=%t, audit=%t)",
		mcpServer.acl.OperatorRole,
		mcpServer.acl.AllowVoteSubmit,
		mcpServer.acl.AllowTally,
		mcpServer.acl.AllowConfigMutation,
		mcpServer.acl.AllowAuditLogs,
	)

	mcpServer.Serve(mcpServer.in, mcpServer.out)
}

// Serve processes newline-delimited JSON-RPC 2.0 requests from reader and
// writes responses to writer. It is injectable for tests.
func (s *PemiraMCPServer) Serve(reader io.Reader, writer io.Writer) {
	s.in = reader
	s.out = writer

	buf := bufio.NewReader(reader)
	for {
		line, err := buf.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return
			}
			s.logger.Printf("Error membaca input: %v", err)
			return
		}
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error: "+err.Error())
			continue
		}

		// Notifikasi (tanpa ID) tidak memerlukan response.
		if req.ID == nil {
			continue
		}

		s.handleRequest(req)
	}
}

func (s *PemiraMCPServer) handleRequest(req JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		s.sendResult(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{},
				"resources": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "pemira-2026-mcp",
				"version": "1.1.0",
			},
		})

	case "ping":
		s.sendResult(req.ID, map[string]interface{}{})

	case "tools/list":
		s.sendResult(req.ID, map[string]interface{}{
			"tools": s.getToolsList(),
		})

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params: "+err.Error())
			return
		}
		res := s.executeTool(params)
		s.sendResult(req.ID, res)

	case "resources/list":
		s.sendResult(req.ID, map[string]interface{}{
			"resources": []map[string]interface{}{
				{
					"uri":         "pemira://config/election",
					"name":        "Jadwal & Konfigurasi Pemilu (ElectionConfig)",
					"description": "Status jadwal mulai/selesai pemilihan dan status emergency pause saat ini.",
					"mimeType":    "application/json",
				},
				{
					"uri":         "pemira://stats/turnout",
					"name":        "Statistik Kehadiran Pemilih (Real-Time Turnout)",
					"description": "Jumlah total DPT, jumlah suara masuk, dan persentase partisipasi tanpa membocorkan pilihan.",
					"mimeType":    "application/json",
				},
				{
					"uri":         "pemira://booth/chairs",
					"name":        "Daftar Kursi & Pasangan Calon Aktif",
					"description": "Daftar seluruh kursi pemilihan (BEM, MPM, HMJ) beserta visi, misi, dan program kerja paslon.",
					"mimeType":    "application/json",
				},
			},
		})

	case "resources/read":
		var params ReadResourceParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params: "+err.Error())
			return
		}
		res, err := s.readResource(params.URI)
		if err != nil {
			s.sendError(req.ID, -32602, err.Error())
			return
		}
		s.sendResult(req.ID, res)

	default:
		s.sendError(req.ID, -32601, "Method not found: "+req.Method)
	}
}

func (s *PemiraMCPServer) getToolsList() []MCPTool {
	return []MCPTool{
		{
			Name:        "pemira_health_check",
			Description: "Memeriksa status kesehatan server database PostgreSQL, Redis lock service, dan jadwal ElectionConfig.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "pemira_get_all_chairs",
			Description: "Mengambil daftar seluruh kursi pemilihan aktif (BEM, MPM, HMJ) beserta daftar pasangan calon (CandidateGroup).",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "pemira_get_eligible_candidates",
			Description: "Mengambil daftar kursi dan pasangan calon yang berhak dipilih berdasarkan jurusan (major) mahasiswa.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"major": map[string]interface{}{
						"type":        "string",
						"description": "Nama jurusan mahasiswa, contoh: 'Teknik Informatika dan Komputer' atau 'Teknik Elektro'",
					},
				},
				"required": []string{"major"},
			},
		},
		{
			Name:        "pemira_lookup_voter",
			Description: "Mencari data mahasiswa di Daftar Pemilih Tetap (DPT) berdasarkan email kampus atau voter ID. Memerlukan MCP_ALLOW_LOOKUP_VOTER=true dan role voter/admin/kpu/bawaslu.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"email": map[string]interface{}{
						"type":        "string",
						"description": "Email kampus mahasiswa (contoh: 13523001@stu.pnj.ac.id)",
					},
					"voterId": map[string]interface{}{
						"type":        "string",
						"description": "ID pemilih (opsional jika email diisi)",
					},
				},
			},
		},
		{
			Name:        "pemira_dev_mock_login",
			Description: "Membuat atau mengambil akun pemilih simulasi (dev) dan men-generate token JWT. Ditolak di production.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"email": map[string]interface{}{
						"type":        "string",
						"description": "Email mahasiswa uji coba (wajib)",
					},
					"nim": map[string]interface{}{
						"type":        "string",
						"description": "NIM mahasiswa (default: 13523001)",
					},
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Nama lengkap mahasiswa",
					},
					"major": map[string]interface{}{
						"type":        "string",
						"description": "Nama jurusan (default: Teknik Informatika dan Komputer)",
					},
				},
				"required": []string{"email"},
			},
		},
		{
			Name:        "pemira_submit_vote",
			Description: "Mengirimkan surat suara ke kotak suara digital secara atomik (Zero-Linkability + Redis SETNX + Conditional SQL Update) dan menghasilkan HMAC-SHA256 Receipt. Memerlukan MCP_ALLOW_VOTE_SUBMIT=true dan role voter/admin.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"voterId": map[string]interface{}{
						"type":        "string",
						"description": "ID unik pemilih yang melakukan pencoblosan",
					},
					"turnstileToken": map[string]interface{}{
						"type":        "string",
						"description": "Token Cloudflare Turnstile (gunakan 'dummy-token-dev' untuk dev)",
					},
					"votes": map[string]interface{}{
						"type":        "array",
						"description": "Daftar pilihan per kursi pemilihan",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"chairId": map[string]interface{}{
									"type":        "integer",
									"description": "ID kursi pemilihan",
								},
								"candidateGroupId": map[string]interface{}{
									"type":        "string",
									"description": "UUID pasangan calon yang dipilih",
								},
							},
							"required": []string{"chairId", "candidateGroupId"},
						},
					},
				},
				"required": []string{"voterId", "votes"},
			},
		},
		{
			Name:        "pemira_verify_receipt",
			Description: "Memverifikasi keabsahan tanda terima suara digital (HMAC-SHA256 Receipt Token).",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"receiptCode": map[string]interface{}{
						"type":        "string",
						"description": "Kode bukti tanda terima suara (contoh: VOTE-RCPT-...)",
					},
				},
				"required": []string{"receiptCode"},
			},
		},
		{
			Name:        "pemira_get_turnout",
			Description: "Mengambil statistik kehadiran pemilih (turnout) secara real-time tanpa membuka isi kotak suara.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "pemira_get_tally_results",
			Description: "Membuka kotak suara dan menghitung hasil perolehan suara. Wajib melewati Time-Lock dan memiliki quorum secret KPU & Bawaslu yang dikonfigurasi di environment server (bukan parameter tool). Memerlukan MCP_ALLOW_TALLY=true dan role admin/kpu/bawaslu.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "pemira_get_audit_logs",
			Description: "Mengambil riwayat jejak audit aktivitas sistem dan panitia (AuditLog). Memerlukan MCP_ALLOW_AUDIT_LOGS=true dan role admin/bawaslu.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Jumlah maksimal baris log yang diambil (default: 50)",
					},
				},
			},
		},
		{
			Name:        "pemira_manage_election_config",
			Description: "Melihat atau memperbarui konfigurasi window waktu pemilihan (StartTime, EndTime, IsEmergencyPaused) dan mencatatnya ke AuditLog. Memerlukan MCP_ALLOW_CONFIG_MUTATION=true dan role admin.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type":        "string",
						"description": "Judul pemilihan (opsional)",
					},
					"startTimeRFC3339": map[string]interface{}{
						"type":        "string",
						"description": "Waktu mulai dalam format RFC3339 (opsional, contoh: 2026-12-10T08:00:00+07:00)",
					},
					"endTimeRFC3339": map[string]interface{}{
						"type":        "string",
						"description": "Waktu selesai dalam format RFC3339 (opsional, contoh: 2026-12-11T17:00:00+07:00)",
					},
					"isEmergencyPaused": map[string]interface{}{
						"type":        "boolean",
						"description": "Status penghentian darurat bilik suara (opsional)",
					},
				},
			},
		},
	}
}

func (s *PemiraMCPServer) authorize(tool string) error {
	switch tool {
	case "pemira_lookup_voter":
		if !s.acl.AllowLookupVoter || !s.acl.hasRole("voter", "admin", "kpu", "bawaslu") {
			return fmt.Errorf("role '%s' tidak diizinkan atau MCP_ALLOW_LOOKUP_VOTER belum aktif", s.acl.OperatorRole)
		}
	case "pemira_submit_vote":
		if !s.acl.AllowVoteSubmit || !s.acl.hasRole("voter", "admin") {
			return fmt.Errorf("role '%s' tidak diizinkan atau MCP_ALLOW_VOTE_SUBMIT belum aktif", s.acl.OperatorRole)
		}
	case "pemira_get_tally_results":
		if !s.acl.AllowTally || !s.acl.hasRole("admin", "kpu", "bawaslu") {
			return fmt.Errorf("role '%s' tidak diizinkan atau MCP_ALLOW_TALLY belum aktif", s.acl.OperatorRole)
		}
	case "pemira_get_audit_logs":
		if !s.acl.AllowAuditLogs || !s.acl.hasRole("admin", "bawaslu") {
			return fmt.Errorf("role '%s' tidak diizinkan atau MCP_ALLOW_AUDIT_LOGS belum aktif", s.acl.OperatorRole)
		}
	case "pemira_manage_election_config":
		if !s.acl.AllowConfigMutation || !s.acl.hasRole("admin") {
			return fmt.Errorf("role '%s' tidak diizinkan atau MCP_ALLOW_CONFIG_MUTATION belum aktif", s.acl.OperatorRole)
		}
	case "pemira_dev_mock_login":
		if !s.acl.AllowMockLogin {
			return fmt.Errorf("MCP_ALLOW_MOCK_LOGIN tidak aktif")
		}
	}
	return nil
}

func (s *PemiraMCPServer) executeTool(params CallToolParams) MCPToolResult {
	if err := s.authorize(params.Name); err != nil {
		s.recordAudit("MCP_TOOL_DENIED", fmt.Sprintf(`{"tool":%q,"role":%q}`, params.Name, s.acl.OperatorRole))
		return s.errorToolResult("FORBIDDEN: " + err.Error())
	}

	switch params.Name {
	case "pemira_health_check":
		electionCfg, _ := s.auditRepo.GetElectionConfig()
		redisConnected := s.redisService != nil && s.redisService.Client != nil
		return s.jsonToolResult(map[string]interface{}{
			"status":         "healthy",
			"service":        "pemira-2026-mcp",
			"engine":         "Zero-Linkability Ballot Engine",
			"serverEnv":      s.cfg.ServerEnv,
			"operatorRole":   s.acl.OperatorRole,
			"redisConnected": redisConnected,
			"electionConfig": electionCfg,
			"timestamp":      time.Now().Format(time.RFC3339),
		})

	case "pemira_get_all_chairs":
		chairs, err := s.chairRepo.FindAllActive()
		if err != nil {
			return s.errorToolResult("Gagal mengambil daftar kursi: " + err.Error())
		}
		return s.jsonToolResult(chairs)

	case "pemira_get_eligible_candidates":
		var args struct {
			Major string `json:"major"`
		}
		if err := unmarshalArgs(params.Arguments, &args); err != nil || args.Major == "" {
			return s.errorToolResult("Parameter 'major' wajib diisi")
		}
		chairs, err := s.chairRepo.FindEligibleForMajor(args.Major)
		if err != nil {
			return s.errorToolResult("Gagal mengambil kandidat untuk jurusan " + args.Major + ": " + err.Error())
		}
		return s.jsonToolResult(chairs)

	case "pemira_lookup_voter":
		var args struct {
			Email   string `json:"email"`
			VoterID string `json:"voterId"`
		}
		_ = unmarshalArgs(params.Arguments, &args)
		var voter *domain.Voter
		var err error
		if args.Email != "" {
			voter, err = s.voterRepo.FindByEmail(strings.ToLower(strings.TrimSpace(args.Email)))
		} else if args.VoterID != "" {
			voter, err = s.voterRepo.FindByID(args.VoterID)
		} else {
			return s.errorToolResult("Harap sertakan 'email' atau 'voterId'")
		}
		if err != nil || voter == nil {
			return s.errorToolResult("Pemilih tidak ditemukan di DPT")
		}
		eligibleChairs, _ := s.chairRepo.FindEligibleForMajor(voter.Major)
		return s.jsonToolResult(map[string]interface{}{
			"voter": map[string]interface{}{
				"id":         voter.ID,
				"nim":        voter.NIM,
				"fullName":   voter.FullName,
				"email":      voter.Email,
				"faculty":    voter.Faculty,
				"major":      voter.Major,
				"generation": voter.Generation,
				"role":       voter.Role,
				"hasVoted":   voter.HasVoted,
			},
			"eligibleChairs": eligibleChairs,
		})

	case "pemira_dev_mock_login":
		if s.cfg.ServerEnv == "production" {
			return s.errorToolResult("FORBIDDEN: Mock login dinonaktifkan di production")
		}
		var args struct {
			Email string `json:"email"`
			NIM   string `json:"nim"`
			Name  string `json:"name"`
			Major string `json:"major"`
		}
		if err := unmarshalArgs(params.Arguments, &args); err != nil || args.Email == "" {
			return s.errorToolResult("Parameter 'email' wajib diisi")
		}
		if args.NIM == "" {
			args.NIM = "13523001"
		}
		if args.Name == "" {
			args.Name = "Mahasiswa Uji Coba"
		}
		if args.Major == "" {
			args.Major = "Teknik Informatika dan Komputer"
		}
		voter, err := s.authService.CreateMockVoterIfNotExist(args.Email, args.NIM, args.Name, args.Major)
		if err != nil {
			return s.errorToolResult("Gagal membuat/mengambil mock voter: " + err.Error())
		}
		jwtToken, err := s.authService.GenerateJWT(voter)
		if err != nil {
			return s.errorToolResult("Gagal men-generate JWT: " + err.Error())
		}
		return s.jsonToolResult(map[string]interface{}{
			"token": jwtToken,
			"voter": voter,
		})

	case "pemira_submit_vote":
		var args struct {
			VoterID        string                  `json:"voterId"`
			TurnstileToken string                  `json:"turnstileToken"`
			Votes          []domain.SingleVoteItem `json:"votes"`
		}
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return s.errorToolResult("Payload suara tidak valid: " + err.Error())
		}
		if args.VoterID == "" || len(args.Votes) == 0 {
			return s.errorToolResult("Parameter 'voterId' dan 'votes' (minimal 1 pilihan) wajib diisi")
		}
		req := domain.SubmitVoteRequest{
			TurnstileToken: args.TurnstileToken,
			Votes:          args.Votes,
		}
		res, err := s.voteService.SubmitVote(context.Background(), args.VoterID, req)
		if err != nil {
			return s.errorToolResult("Gagal memproses suara: " + err.Error())
		}
		return s.jsonToolResult(res)

	case "pemira_verify_receipt":
		var args struct {
			ReceiptCode string `json:"receiptCode"`
		}
		if err := unmarshalArgs(params.Arguments, &args); err != nil || args.ReceiptCode == "" {
			return s.errorToolResult("Parameter 'receiptCode' wajib diisi")
		}
		valid, _ := s.voteService.GetReceiptVerification(args.ReceiptCode)
		return s.jsonToolResult(map[string]interface{}{
			"valid":   valid,
			"receipt": args.ReceiptCode,
			"status":  "Suara telah terverifikasi tersimpan di dalam kotak suara digital.",
		})

	case "pemira_get_turnout":
		total, voted, err := s.voterRepo.GetTurnoutStats()
		if err != nil {
			return s.errorToolResult("Gagal membaca statistik kehadiran: " + err.Error())
		}
		percentage := 0.0
		if total > 0 {
			percentage = (float64(voted) / float64(total)) * 100.0
		}
		return s.jsonToolResult(map[string]interface{}{
			"totalVoters": total,
			"votedCount":  voted,
			"notVoted":    total - voted,
			"percentage":  percentage,
			"timestamp":   time.Now().Format(time.RFC3339),
		})

	case "pemira_get_tally_results":
		// FAIL-CLOSED: a database read error or missing config must never fall
		// through to the tally query.
		electionCfg, err := s.auditRepo.GetElectionConfig()
		if err != nil || electionCfg == nil {
			s.recordAudit("TALLY_QUORUM_FAILED", `{"reason":"election_config_unavailable"}`)
			return s.errorToolResult("FAIL-CLOSED: Gagal membaca konfigurasi jendela pemilu. Rekapitulasi ditolak.")
		}
		if time.Now().Before(electionCfg.EndTime) {
			s.recordAudit("TALLY_QUORUM_FAILED", `{"reason":"election_not_ended"}`)
			return s.errorToolResult(fmt.Sprintf(
				"QUARANTINED: Rekapitulasi suara masih dikarantina hingga waktu pemilihan berakhir (%s).",
				electionCfg.EndTime.Format(time.RFC3339),
			))
		}

		// Quorum secrets come exclusively from the server environment/secret
		// manager. They are never accepted as tool arguments and never logged.
		kpuSecret := strings.TrimSpace(s.cfg.KPUMasterSecret)
		bawasluSecret := strings.TrimSpace(s.cfg.BawasluAuditSecret)
		if kpuSecret == "" || bawasluSecret == "" {
			s.recordAudit("TALLY_QUORUM_FAILED", `{"reason":"quorum_not_configured"}`)
			return s.errorToolResult("UNAUTHORIZED: Quorum secret KPU/Bawaslu belum dikonfigurasi pada environment server.")
		}
		if subtle.ConstantTimeCompare([]byte(kpuSecret), []byte(bawasluSecret)) == 1 {
			s.recordAudit("TALLY_QUORUM_FAILED", `{"reason":"quorum_secrets_identical"}`)
			return s.errorToolResult("UNAUTHORIZED: Quorum KPU dan Bawaslu tidak boleh identik.")
		}

		results, err := s.ballotRepo.GetTallyResults()
		if err != nil {
			s.recordAudit("TALLY_QUORUM_FAILED", `{"reason":"tally_query_failed"}`)
			return s.errorToolResult("Gagal melakukan tally suara: " + err.Error())
		}

		chairTotals := make(map[uint]int64)
		for _, r := range results {
			chairTotals[r.ChairID] += r.TotalVotes
		}
		for i := range results {
			total := chairTotals[results[i].ChairID]
			if total > 0 {
				results[i].Percentage = math.Round((float64(results[i].TotalVotes)/float64(total))*10000) / 100
			} else {
				results[i].Percentage = 0
			}
		}

		s.recordAudit("TALLY_UNLOCKED", fmt.Sprintf(`{"chairs_count":%d,"total_results":%d}`, len(chairTotals), len(results)))
		return s.jsonToolResult(map[string]interface{}{
			"success":    true,
			"status":     "QUORUM_UNLOCKED",
			"tally":      results,
			"unlockedAt": time.Now().Format(time.RFC3339),
		})

	case "pemira_get_audit_logs":
		var args struct {
			Limit int `json:"limit"`
		}
		_ = unmarshalArgs(params.Arguments, &args)
		if args.Limit <= 0 || args.Limit > 500 {
			args.Limit = 50
		}
		logs, err := s.auditRepo.GetRecentLogs(args.Limit)
		if err != nil {
			return s.errorToolResult("Gagal mengambil log audit: " + err.Error())
		}
		return s.jsonToolResult(logs)

	case "pemira_manage_election_config":
		var args struct {
			Title             *string `json:"title"`
			StartTimeRFC3339  *string `json:"startTimeRFC3339"`
			EndTimeRFC3339    *string `json:"endTimeRFC3339"`
			IsEmergencyPaused *bool   `json:"isEmergencyPaused"`
		}
		_ = unmarshalArgs(params.Arguments, &args)
		cfg, err := s.auditRepo.GetElectionConfig()
		if err != nil || cfg == nil {
			return s.errorToolResult("Gagal membaca ElectionConfig dari database")
		}
		changed := make([]string, 0, 4)
		if args.Title != nil {
			title := strings.TrimSpace(*args.Title)
			if title == "" {
				return s.errorToolResult("Title tidak boleh kosong")
			}
			cfg.Title = title
			changed = append(changed, "title")
		}
		if args.StartTimeRFC3339 != nil && strings.TrimSpace(*args.StartTimeRFC3339) != "" {
			t, err := time.Parse(time.RFC3339, strings.TrimSpace(*args.StartTimeRFC3339))
			if err != nil {
				return s.errorToolResult("Format startTimeRFC3339 tidak valid (gunakan RFC3339): " + err.Error())
			}
			cfg.StartTime = t
			changed = append(changed, "startTime")
		}
		if args.EndTimeRFC3339 != nil && strings.TrimSpace(*args.EndTimeRFC3339) != "" {
			t, err := time.Parse(time.RFC3339, strings.TrimSpace(*args.EndTimeRFC3339))
			if err != nil {
				return s.errorToolResult("Format endTimeRFC3339 tidak valid (gunakan RFC3339): " + err.Error())
			}
			cfg.EndTime = t
			changed = append(changed, "endTime")
		}
		if args.IsEmergencyPaused != nil {
			cfg.IsEmergencyPaused = *args.IsEmergencyPaused
			changed = append(changed, "isEmergencyPaused")
		}
		if len(changed) == 0 {
			return s.jsonToolResult(cfg)
		}
		if !cfg.StartTime.Before(cfg.EndTime) {
			return s.errorToolResult("startTime harus lebih awal daripada endTime")
		}

		cfg.UpdatedAt = time.Now()
		if err := s.auditRepo.UpdateElectionConfig(cfg); err != nil {
			return s.errorToolResult("Gagal menyimpan perubahan ElectionConfig: " + err.Error())
		}

		service.InvalidateElectionConfigCache(context.Background(), s.redisService)
		s.recordAudit("MCP_CONFIG_UPDATE", fmt.Sprintf(`{"changed":%q}`, strings.Join(changed, ",")))
		return s.jsonToolResult(cfg)

	default:
		return s.errorToolResult("Tool tidak dikenal: " + params.Name)
	}
}

func (s *PemiraMCPServer) readResource(uri string) (interface{}, error) {
	switch uri {
	case "pemira://config/election":
		if !s.acl.hasRole("admin", "kpu", "bawaslu") {
			return nil, fmt.Errorf("FORBIDDEN: resource konfigurasi pemilu memerlukan role admin/kpu/bawaslu")
		}
		cfg, err := s.auditRepo.GetElectionConfig()
		if err != nil {
			return nil, fmt.Errorf("gagal membaca konfigurasi pemilu: %w", err)
		}
		b, _ := json.MarshalIndent(cfg, "", "  ")
		return map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      uri,
					"mimeType": "application/json",
					"text":     string(b),
				},
			},
		}, nil

	case "pemira://stats/turnout":
		total, voted, err := s.voterRepo.GetTurnoutStats()
		if err != nil {
			return nil, fmt.Errorf("gagal membaca turnout: %w", err)
		}
		pct := 0.0
		if total > 0 {
			pct = (float64(voted) / float64(total)) * 100.0
		}
		b, _ := json.MarshalIndent(map[string]interface{}{
			"totalVoters": total,
			"votedCount":  voted,
			"notVoted":    total - voted,
			"percentage":  pct,
			"timestamp":   time.Now().Format(time.RFC3339),
		}, "", "  ")
		return map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      uri,
					"mimeType": "application/json",
					"text":     string(b),
				},
			},
		}, nil

	case "pemira://booth/chairs":
		chairs, err := s.chairRepo.FindAllActive()
		if err != nil {
			return nil, fmt.Errorf("gagal membaca daftar kursi: %w", err)
		}
		b, _ := json.MarshalIndent(chairs, "", "  ")
		return map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"uri":      uri,
					"mimeType": "application/json",
					"text":     string(b),
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("resource URI tidak ditemukan: %s", uri)
	}
}

func (s *PemiraMCPServer) recordAudit(action, payload string) {
	_ = s.auditRepo.LogAction(&domain.AuditLog{
		ActorID:   "mcp:" + s.acl.OperatorRole,
		ActorRole: s.acl.OperatorRole,
		Action:    action,
		IPAddress: "127.0.0.1",
		UserAgent: "pemira-2026-mcp/1.1.0",
		Payload:   payload,
		CreatedAt: time.Now(),
	})
}

func (s *PemiraMCPServer) jsonToolResult(data interface{}) MCPToolResult {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return s.errorToolResult("Gagal melakukan serialisasi output: " + err.Error())
	}
	return MCPToolResult{
		Content: []MCPContent{
			{
				Type: "text",
				Text: string(b),
			},
		},
	}
}

func (s *PemiraMCPServer) errorToolResult(msg string) MCPToolResult {
	return MCPToolResult{
		IsError: true,
		Content: []MCPContent{
			{
				Type: "text",
				Text: msg,
			},
		},
	}
}

func (s *PemiraMCPServer) sendResult(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	b, _ := json.Marshal(resp)
	fmt.Fprintf(s.out, "%s\n", string(b))
}

func (s *PemiraMCPServer) sendError(id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	b, _ := json.Marshal(resp)
	fmt.Fprintf(s.out, "%s\n", string(b))
}

func unmarshalArgs(raw json.RawMessage, v interface{}) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return json.Unmarshal(raw, v)
}
