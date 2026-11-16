package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"pemira-backend/internal/config"
	"pemira-backend/internal/database"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/repository"
	"pemira-backend/internal/service"
	"pemira-backend/pkg/redis"
	"testing"
	"time"

	"gorm.io/gorm"
)

func newTestMCPServer(t *testing.T, cfg *config.Config, acl MCPAccessControl) (*PemiraMCPServer, *gorm.DB) {
	t.Helper()

	if cfg == nil {
		cfg = &config.Config{
			ServerEnv:          "test",
			DBDriver:           "sqlite",
			DBName:             ":memory:",
			DBMaxOpenConns:     5,
			DBMaxIdleConns:     2,
			DBConnMaxLifetime:  time.Minute,
			JWTSecret:          "test-jwt-secret-long-enough-32-chars-at-least!",
			ReceiptSecretKey:   "test-receipt-secret-key-32-chars-long!",
			ReceiptSalt:        "test-receipt-salt-16-chars!",
			KPUMasterSecret:    "kpu-quorum-secret-16chars!",
			BawasluAuditSecret: "bawaslu-quorum-secret-16chars!",
		}
	} else {
		if cfg.DBDriver == "" {
			cfg.DBDriver = "sqlite"
		}
		if cfg.DBName == "" {
			cfg.DBName = ":memory:"
		}
		if cfg.DBMaxOpenConns == 0 {
			cfg.DBMaxOpenConns = 5
		}
		if cfg.DBMaxIdleConns == 0 {
			cfg.DBMaxIdleConns = 2
		}
		if cfg.DBConnMaxLifetime == 0 {
			cfg.DBConnMaxLifetime = time.Minute
		}
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect test db: %v", err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	voterRepo := repository.NewVoterRepository(db)
	ballotRepo := repository.NewBallotRepository(db)
	chairRepo := repository.NewChairRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	redisService := &redis.RedisService{Client: nil}

	server := &PemiraMCPServer{
		cfg:          cfg,
		voterRepo:    voterRepo,
		ballotRepo:   ballotRepo,
		chairRepo:    chairRepo,
		auditRepo:    auditRepo,
		authService:  service.NewAuthService(voterRepo, redisService, cfg),
		voteService:  service.NewVoteService(db, voterRepo, ballotRepo, chairRepo, auditRepo, redisService, cfg),
		redisService: redisService,
		acl:          acl,
		logger:       log.New(io.Discard, "", 0),
		in:           bytes.NewReader(nil),
		out:          &bytes.Buffer{},
	}
	return server, db
}

func permissiveACL() MCPAccessControl {
	return MCPAccessControl{
		OperatorRole:        "admin",
		AllowVoteSubmit:     true,
		AllowTally:          true,
		AllowConfigMutation: true,
		AllowAuditLogs:      true,
		AllowLookupVoter:    true,
		AllowMockLogin:      true,
	}
}

func TestMCPInitializeAndToolsListOverJSONRPC(t *testing.T) {
	server, _ := newTestMCPServer(t, nil, permissiveACL())
	var out bytes.Buffer
	server.out = &out

	server.handleRequest(JSONRPCRequest{JSONRPC: "2.0", ID: float64(1), Method: "initialize"})
	if out.Len() == 0 {
		t.Fatal("expected initialize response")
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON-RPC response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}

	out.Reset()
	server.handleRequest(JSONRPCRequest{JSONRPC: "2.0", ID: float64(2), Method: "tools/list"})
	var listResp struct {
		Result struct {
			Tools []MCPTool `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &listResp); err != nil {
		t.Fatalf("invalid tools/list response: %v", err)
	}
	if len(listResp.Result.Tools) == 0 {
		t.Fatal("expected at least one MCP tool")
	}
}

func TestMCPToolsListDoesNotExposeQuorumSecrets(t *testing.T) {
	server, _ := newTestMCPServer(t, nil, permissiveACL())
	b, err := json.Marshal(server.getToolsList())
	if err != nil {
		t.Fatalf("failed to marshal tools: %v", err)
	}
	text := string(b)
	for _, forbidden := range []string{"kpuSecretKey", "bawasluSecretKey", "KPU_MASTER_SECRET", "BAWASLU_AUDIT_SECRET"} {
		if bytes.Contains(b, []byte(forbidden)) {
			t.Errorf("tool schema must not expose quorum secret parameter %q: %s", forbidden, text)
		}
	}
}

func TestMCPMockLoginRejectedInProduction(t *testing.T) {
	cfg := &config.Config{
		ServerEnv:          "production",
		JWTSecret:          "prod-jwt-secret-long-enough-32-chars!",
		ReceiptSecretKey:   "prod-receipt-secret-long-enough-32!",
		ReceiptSalt:        "prod-receipt-salt-16!",
		KPUMasterSecret:    "prod-kpu-quorum-secret-16!",
		BawasluAuditSecret: "prod-bawaslu-quorum-secret-16!",
	}
	server, _ := newTestMCPServer(t, cfg, permissiveACL())

	res := server.executeTool(CallToolParams{
		Name:      "pemira_dev_mock_login",
		Arguments: json.RawMessage(`{"email":"test@stu.pnj.ac.id"}`),
	})
	if !res.IsError {
		t.Fatalf("expected mock login to be rejected in production")
	}
	if !bytes.Contains([]byte(res.Content[0].Text), []byte("FORBIDDEN")) {
		t.Errorf("expected FORBIDDEN error, got: %s", res.Content[0].Text)
	}
}

func TestMCPTallyFailClosedWhenElectionConfigMissing(t *testing.T) {
	server, _ := newTestMCPServer(t, nil, permissiveACL())

	res := server.executeTool(CallToolParams{Name: "pemira_get_tally_results"})
	if !res.IsError {
		t.Fatalf("expected tally to fail closed when ElectionConfig is missing")
	}
	if !bytes.Contains([]byte(res.Content[0].Text), []byte("FAIL-CLOSED")) {
		t.Errorf("expected FAIL-CLOSED message, got: %s", res.Content[0].Text)
	}
}

func TestMCPTallyRequiresConfiguredQuorumSecrets(t *testing.T) {
	cfg := &config.Config{
		ServerEnv:        "test",
		JWTSecret:        "test-jwt-secret-long-enough-32-chars-at-least!",
		ReceiptSecretKey: "test-receipt-secret-key-32-chars-long!",
		ReceiptSalt:      "test-receipt-salt-16-chars!",
		// Quorum secrets intentionally empty.
	}
	server, db := newTestMCPServer(t, cfg, permissiveACL())

	now := time.Now()
	db.Create(&domain.ElectionConfig{
		Title:     "Pemira Test",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(-1 * time.Hour),
	})

	res := server.executeTool(CallToolParams{Name: "pemira_get_tally_results"})
	if !res.IsError {
		t.Fatalf("expected tally to reject missing quorum secrets")
	}
	if !bytes.Contains([]byte(res.Content[0].Text), []byte("UNAUTHORIZED")) {
		t.Errorf("expected UNAUTHORIZED message, got: %s", res.Content[0].Text)
	}
}

func TestMCPPrivilegedToolsDeniedForReadonlyRole(t *testing.T) {
	acl := MCPAccessControl{OperatorRole: "readonly"}
	server, _ := newTestMCPServer(t, nil, acl)

	for _, tool := range []string{
		"pemira_submit_vote",
		"pemira_get_tally_results",
		"pemira_get_audit_logs",
		"pemira_manage_election_config",
		"pemira_lookup_voter",
	} {
		res := server.executeTool(CallToolParams{
			Name:      tool,
			Arguments: json.RawMessage(`{}`),
		})
		if !res.IsError {
			t.Errorf("expected tool %s to be denied for readonly role", tool)
			continue
		}
		if !bytes.Contains([]byte(res.Content[0].Text), []byte("FORBIDDEN")) {
			t.Errorf("expected FORBIDDEN for %s, got: %s", tool, res.Content[0].Text)
		}
	}
}

func TestMCPManageElectionConfigWritesAuditLog(t *testing.T) {
	server, db := newTestMCPServer(t, nil, permissiveACL())

	now := time.Now()
	db.Create(&domain.ElectionConfig{
		Title:     "Pemira Test",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(2 * time.Hour),
	})

	res := server.executeTool(CallToolParams{
		Name:      "pemira_manage_election_config",
		Arguments: json.RawMessage(`{"title":"Pemira Updated","isEmergencyPaused":true}`),
	})
	if res.IsError {
		t.Fatalf("expected config update to succeed, got: %s", res.Content[0].Text)
	}

	var cfg domain.ElectionConfig
	if err := db.First(&cfg).Error; err != nil {
		t.Fatalf("failed to read election config: %v", err)
	}
	if cfg.Title != "Pemira Updated" || !cfg.IsEmergencyPaused {
		t.Errorf("config update not persisted: %+v", cfg)
	}

	var auditCount int64
	db.Model(&domain.AuditLog{}).Where("action = ?", "MCP_CONFIG_UPDATE").Count(&auditCount)
	if auditCount == 0 {
		t.Error("expected MCP_CONFIG_UPDATE audit log")
	}
}
