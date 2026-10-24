package sqlite

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// BackupTo 用 SQLite 的 VACUUM INTO 生成一致性快照。
//
// 为什么不是 cp：WAL 模式下直接复制 .db 会丢掉未 checkpoint 的页，
// 拿到一个"能打开但少数据"的文件——比备份失败更糟。
//
// 目标文件必须不存在（SQLite 的行为）；这里提前检查，给出可读错误。
func (s *Store) BackupTo(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("sqlite: backup path required")
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("sqlite: backup target %s already exists", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("sqlite: stat %s: %w", path, err)
	}
	// VACUUM 不接受参数占位符，只能拼接；单引号按 SQL 规则转义。
	quoted := "'" + strings.ReplaceAll(path, "'", "''") + "'"
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO "+quoted); err != nil {
		return fmt.Errorf("sqlite: backup to %s: %w", path, err)
	}
	return nil
}
