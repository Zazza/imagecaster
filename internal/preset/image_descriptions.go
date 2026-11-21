package preset

import (
	"database/sql"
	"errors"
)

func (d *DB) GetImageDescription(hash, mode string) (string, bool, error) {
	var text string
	err := d.db.QueryRow(`SELECT text FROM image_descriptions WHERE hash = ? AND mode = ?`, hash, mode).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

func (d *DB) SaveImageDescription(hash, mode, text string) error {
	_, err := d.db.Exec(`INSERT INTO image_descriptions (hash, mode, text) VALUES (?, ?, ?)
ON CONFLICT(hash, mode) DO UPDATE SET text = excluded.text, created_at = datetime('now')`, hash, mode, text)
	return err
}
