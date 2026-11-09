package sqlite

import (
	"context"
	"database/sql"
)

// 迁移按版本递增；已存在的库从 user_version 继续往上跑。
// v3 需要在 Go 中计算历史哈希，因此迁移不再局限于纯 DDL。
//
// 约定：**只追加，不改历史版本**。改了 v1 的语句会让老库和新库分叉。
type migration struct {
	version int
	apply   func(context.Context, *sql.Tx) error
}

func ddlMigration(stmts []string) func(context.Context, *sql.Tx) error {
	return func(ctx context.Context, tx *sql.Tx) error {
		for _, stmt := range stmts {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return err
			}
		}
		return nil
	}
}

var migrations = []migration{
	{1, ddlMigration(schemaV1)},
	{2, ddlMigration(schemaV2)},
	{3, migrateAuditChain},
}

// latestVersion 是最新 schema 版本，必须等于 migrations 里最大的 version。
func latestVersion() int {
	return migrations[len(migrations)-1].version
}

// schemaV1 是 v1 的完整 DDL，按顺序执行（迁移在一个事务里跑完）。
//
// 两个关键点（都来自实测，见 04-技术路线/数据模型 与 检索验证-S8）：
//  1. memories 额外存一列 fts_text（CJK 二元组文本），FTS5 用 external content
//     指向这一列 —— 这样触发器可以直接引用 new.fts_text/old.fts_text，
//     不需要在 SQL 里做中文分词；
//  2. 去重是「部分唯一索引」：只约束 active 状态，软删除后同内容可以重新写入。
var schemaV1 = []string{
	`CREATE TABLE IF NOT EXISTS memories (
		id             TEXT PRIMARY KEY,
		tenant_id      TEXT NOT NULL DEFAULT 'default',
		scope          TEXT NOT NULL DEFAULT 'default',
		content        TEXT NOT NULL,
		fts_text       TEXT NOT NULL,
		content_hash   TEXT NOT NULL,
		kind           TEXT NOT NULL DEFAULT 'note',
		source         TEXT,
		metadata       TEXT,
		status         TEXT NOT NULL DEFAULT 'active',
		created_at     INTEGER NOT NULL,
		updated_at     INTEGER NOT NULL,
		expires_at     INTEGER,
		last_access_at INTEGER,
		access_count   INTEGER NOT NULL DEFAULT 0
	)`,

	`CREATE UNIQUE INDEX IF NOT EXISTS ux_memories_dedup
		ON memories(tenant_id, scope, content_hash) WHERE status = 'active'`,
	`CREATE INDEX IF NOT EXISTS ix_memories_scope
		ON memories(tenant_id, scope, status)`,
	`CREATE INDEX IF NOT EXISTS ix_memories_expires
		ON memories(expires_at) WHERE expires_at IS NOT NULL`,

	`CREATE TABLE IF NOT EXISTS embeddings (
		memory_id  TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
		model      TEXT NOT NULL,
		dim        INTEGER NOT NULL,
		vector     BLOB NOT NULL,
		created_at INTEGER NOT NULL,
		PRIMARY KEY (memory_id, model)
	)`,

	// 外部内容表的列名必须与 memories 中的列名一致（fts_text）
	`CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts
		USING fts5(fts_text, content='memories', content_rowid='rowid')`,

	`CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
		INSERT INTO memories_fts(rowid, fts_text) VALUES (new.rowid, new.fts_text);
	END`,
	`CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, fts_text) VALUES ('delete', old.rowid, old.fts_text);
	END`,
	`CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE OF fts_text ON memories BEGIN
		INSERT INTO memories_fts(memories_fts, rowid, fts_text) VALUES ('delete', old.rowid, old.fts_text);
		INSERT INTO memories_fts(rowid, fts_text) VALUES (new.rowid, new.fts_text);
	END`,
}

// schemaV2 增加审计表（S15）。
//
// 设计要点：
//   - 独立表、不进 FTS、不参与检索：审计是"记录"，不是"记忆"
//   - 索引只建在时间与租户上，够导出用，不做过早优化
//   - UPDATE 触发器让 append-only 成为数据库层约束，而不是口头约定
var schemaV2 = []string{
	`CREATE TABLE IF NOT EXISTS audit_log (
		id           TEXT PRIMARY KEY,
		ts           INTEGER NOT NULL,
		actor        TEXT,
		tenant_id    TEXT NOT NULL,
		scope        TEXT,
		action       TEXT NOT NULL,
		memory_id    TEXT,
		query_hash   TEXT,
		query_len    INTEGER NOT NULL DEFAULT 0,
		result_count INTEGER NOT NULL DEFAULT 0,
		degraded     INTEGER NOT NULL DEFAULT 0,
		duplicate    INTEGER NOT NULL DEFAULT 0,
		redacted     INTEGER NOT NULL DEFAULT 0,
		dry_run      INTEGER NOT NULL DEFAULT 0,
		detail       TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS ix_audit_ts ON audit_log(ts)`,
	`CREATE INDEX IF NOT EXISTS ix_audit_tenant_ts ON audit_log(tenant_id, ts)`,
	`CREATE INDEX IF NOT EXISTS ix_audit_memory ON audit_log(memory_id) WHERE memory_id IS NOT NULL`,
	`CREATE TRIGGER IF NOT EXISTS audit_no_update BEFORE UPDATE ON audit_log BEGIN
		SELECT RAISE(ABORT, 'audit_log is append-only');
	END`,
}

// v3 重建审计表：旧表的 UPDATE 触发器不允许直接回填 hash，所以迁移事务内
// 先改名、计算并插入新表，再删除旧表。所有步骤要么全部提交，要么全部回滚。
var auditChainPreDDL = []string{
	`DROP TRIGGER IF EXISTS audit_no_update`,
	`ALTER TABLE audit_log RENAME TO audit_log_v2`,
	`CREATE TABLE audit_log (
		seq          INTEGER PRIMARY KEY,
		-- SQLite 的 v2 TEXT PRIMARY KEY 允许 NULL；保留此类历史行。新事件由 AppendAudit 保证非空。
		id           TEXT UNIQUE,
		ts           INTEGER NOT NULL,
		actor        TEXT,
		tenant_id    TEXT NOT NULL,
		scope        TEXT,
		action       TEXT NOT NULL,
		memory_id    TEXT,
		query_hash   TEXT,
		query_len    INTEGER NOT NULL DEFAULT 0,
		result_count INTEGER NOT NULL DEFAULT 0,
		degraded     INTEGER NOT NULL DEFAULT 0,
		duplicate    INTEGER NOT NULL DEFAULT 0,
		redacted     INTEGER NOT NULL DEFAULT 0,
		dry_run      INTEGER NOT NULL DEFAULT 0,
		detail       TEXT,
		prev_hash    TEXT NOT NULL,
		hash         TEXT NOT NULL
	)`,
}

var auditChainPostDDL = []string{
	`DROP TABLE audit_log_v2`,
	`CREATE INDEX ix_audit_ts ON audit_log(ts)`,
	`CREATE INDEX ix_audit_tenant_ts ON audit_log(tenant_id, ts)`,
	`CREATE INDEX ix_audit_memory ON audit_log(memory_id) WHERE memory_id IS NOT NULL`,
	// REPLACE 的隐式 DELETE 在 recursive_triggers=OFF 时可能不触发 DELETE 触发器；
	// 在冲突处理之前直接拒绝同 seq/id 的插入，封堵 INSERT OR REPLACE 绕过。
	`CREATE TRIGGER audit_no_replace BEFORE INSERT ON audit_log
	 WHEN EXISTS(SELECT 1 FROM audit_log WHERE seq = NEW.seq OR id = NEW.id) BEGIN
		SELECT RAISE(ABORT, 'audit_log is append-only');
	END`,
	`CREATE TRIGGER audit_no_update BEFORE UPDATE ON audit_log BEGIN
		SELECT RAISE(ABORT, 'audit_log is append-only');
	END`,
	`CREATE TRIGGER audit_no_delete BEFORE DELETE ON audit_log BEGIN
		SELECT RAISE(ABORT, 'audit_log is append-only');
	END`,
}
