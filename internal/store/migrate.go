package store

// migrate applies the schema. Each statement is idempotent so the same file
// can be reopened across restarts without data loss.
func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS catalog_house (
			id TEXT PRIMARY KEY, code TEXT NOT NULL, name TEXT NOT NULL,
			valid_from INTEGER NOT NULL, valid_to INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS collection_shift (
			id TEXT PRIMARY KEY, house_id TEXT NOT NULL, code TEXT NOT NULL,
			valid_from INTEGER NOT NULL, valid_to INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS fumigation_batch (
			id TEXT PRIMARY KEY, digest TEXT NOT NULL, version INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS incubator_slot (
			id TEXT PRIMARY KEY, code TEXT NOT NULL,
			valid_from INTEGER NOT NULL, valid_to INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS candling_window (
			id TEXT PRIMARY KEY, code TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS device (
			id TEXT PRIMARY KEY, kind TEXT NOT NULL, code TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS person_qualification (
			person_id TEXT NOT NULL, role TEXT NOT NULL,
			valid_from INTEGER NOT NULL, valid_to INTEGER NOT NULL,
			PRIMARY KEY (person_id, role))`,
		`CREATE TABLE IF NOT EXISTS ruleset_version (
			version INTEGER PRIMARY KEY, precisions TEXT NOT NULL, thresholds TEXT NOT NULL)`,

		`CREATE TABLE IF NOT EXISTS incubation_task (
			id TEXT PRIMARY KEY,
			batch_no TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			generation INTEGER NOT NULL DEFAULT 1,
			rule_snapshot INTEGER NOT NULL,
			fumigation_digest TEXT NOT NULL DEFAULT '',
			house_id TEXT NOT NULL DEFAULT '',
			shift_id TEXT NOT NULL DEFAULT '',
			receivers TEXT NOT NULL DEFAULT '[]',
			version INTEGER NOT NULL DEFAULT 1,
			logical_time INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			final_kind TEXT NOT NULL DEFAULT '',
			final_version INTEGER NOT NULL DEFAULT 0)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_task_batch_open
			ON incubation_task(batch_no) WHERE batch_no <> '' AND status NOT IN ('admitted','isolated','cancelled')`,

		`CREATE TABLE IF NOT EXISTS tray_seal (
			task_id TEXT NOT NULL, seal_no TEXT NOT NULL,
			positions TEXT NOT NULL,
			PRIMARY KEY (task_id, seal_no))`,
		`CREATE TABLE IF NOT EXISTS blind_sample (
			task_id TEXT NOT NULL, code TEXT NOT NULL, digest TEXT NOT NULL,
			revealed INTEGER NOT NULL DEFAULT 0, reveal_version INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (task_id, code))`,

		`CREATE TABLE IF NOT EXISTS resource_lease (
			lease_type TEXT NOT NULL, resource_key TEXT NOT NULL,
			task_id TEXT NOT NULL, generation INTEGER NOT NULL,
			acquired_at INTEGER NOT NULL, expires_at INTEGER NOT NULL,
			release_reason TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (lease_type, resource_key))`,

		`CREATE TABLE IF NOT EXISTS candling_entry (
			task_id TEXT NOT NULL, seal_no TEXT NOT NULL, position INTEGER NOT NULL,
			category TEXT NOT NULL, defects TEXT NOT NULL, retest INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (task_id, seal_no, position, version))`,
		`CREATE TABLE IF NOT EXISTS physicochemical_evidence (
			task_id TEXT NOT NULL, seal_no TEXT NOT NULL, position INTEGER NOT NULL,
			kind TEXT NOT NULL, raw INTEGER NOT NULL, derived INTEGER NOT NULL DEFAULT 0,
			version INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (task_id, seal_no, position, kind, version))`,

		`CREATE TABLE IF NOT EXISTS culture_evidence (
			task_id TEXT NOT NULL, well TEXT NOT NULL, colony INTEGER NOT NULL,
			version INTEGER NOT NULL, current INTEGER NOT NULL DEFAULT 1, raw_digest TEXT NOT NULL,
			PRIMARY KEY (task_id, well, version))`,
		`CREATE TABLE IF NOT EXISTS rapid_test_evidence (
			task_id TEXT NOT NULL, well TEXT NOT NULL, ct_value INTEGER NOT NULL,
			version INTEGER NOT NULL, current INTEGER NOT NULL DEFAULT 1, raw_digest TEXT NOT NULL,
			PRIMARY KEY (task_id, well, version))`,
		`CREATE TABLE IF NOT EXISTS retest_case (
			task_id TEXT NOT NULL, generation INTEGER NOT NULL,
			affected_seals TEXT NOT NULL, affected_positions TEXT NOT NULL,
			affected_blinds TEXT NOT NULL, affected_wells TEXT NOT NULL,
			verdict TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (task_id, generation))`,

		`CREATE TABLE IF NOT EXISTS receipt_confirmation (
			task_id TEXT NOT NULL, person_id TEXT NOT NULL, generation INTEGER NOT NULL,
			PRIMARY KEY (task_id, person_id, generation))`,
		`CREATE TABLE IF NOT EXISTS independent_review (
			task_id TEXT NOT NULL, person_id TEXT NOT NULL, generation INTEGER NOT NULL,
			decision TEXT NOT NULL,
			PRIMARY KEY (task_id, person_id, generation))`,
		`CREATE TABLE IF NOT EXISTS operation_dedup (
			task_id TEXT NOT NULL, operation_id TEXT NOT NULL, generation INTEGER NOT NULL,
			content_hash TEXT NOT NULL, response_json TEXT NOT NULL,
			PRIMARY KEY (task_id, operation_id, generation))`,
		`CREATE TABLE IF NOT EXISTS device_attempt (
			task_id TEXT NOT NULL, device_id TEXT NOT NULL, kind TEXT NOT NULL,
			object TEXT NOT NULL, generation INTEGER NOT NULL, attempt INTEGER NOT NULL,
			failure TEXT NOT NULL, next_at INTEGER NOT NULL, pending INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (task_id, device_id, kind, object, generation, attempt))`,

		`CREATE TABLE IF NOT EXISTS swab_seal (
			task_id TEXT NOT NULL, seal_no TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (task_id, seal_no))`,
		`CREATE TABLE IF NOT EXISTS retest_evidence (
			task_id TEXT NOT NULL, generation INTEGER NOT NULL,
			kind TEXT NOT NULL, value INTEGER NOT NULL, version INTEGER NOT NULL,
			PRIMARY KEY (task_id, generation, kind, version))`,

		`CREATE TABLE IF NOT EXISTS final_decision (
			task_id TEXT PRIMARY KEY, kind TEXT NOT NULL, version INTEGER NOT NULL,
			evidence_digest TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS incubation_credential (
			task_id TEXT PRIMARY KEY, number TEXT NOT NULL, version INTEGER NOT NULL)`,

		`CREATE TABLE IF NOT EXISTS audit_log (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL, operation_id TEXT NOT NULL DEFAULT '',
			event TEXT NOT NULL, detail TEXT NOT NULL DEFAULT '',
			logical_time INTEGER NOT NULL, created_at INTEGER NOT NULL)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
