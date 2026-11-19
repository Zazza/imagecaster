package preset

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type StylistPreset struct {
	ID          int64
	Aspect      string
	Name        string
	Prompt      string
	PromptRu    string
	Place       string
	Framing     string
	Places      []string
	Orientation string
}

func (d *DB) ListStylistPresets() ([]StylistPreset, error) {
	rows, err := d.db.Query(`SELECT id, aspect, name, prompt, prompt_ru, place, framing, places, orientation FROM stylist_presets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StylistPreset
	for rows.Next() {
		var p StylistPreset
		var places string
		if err := rows.Scan(&p.ID, &p.Aspect, &p.Name, &p.Prompt, &p.PromptRu, &p.Place, &p.Framing, &places, &p.Orientation); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(places), &p.Places); err != nil {
			return nil, fmt.Errorf("заготовка стилиста %d: места: %w", p.ID, err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) CreateStylistPreset(p *StylistPreset) error {
	places := p.Places
	if places == nil {
		places = []string{}
	}
	raw, err := json.Marshal(places)
	if err != nil {
		return err
	}
	res, err := d.db.Exec(`INSERT INTO stylist_presets (aspect, name, prompt, prompt_ru, place, framing, places, orientation) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Aspect, p.Name, p.Prompt, p.PromptRu, p.Place, p.Framing, string(raw), p.Orientation)
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	return err
}

func (d *DB) RenameStylistPreset(id int64, name string) error {
	return stylistPresetChanged(id, d.db.QueryRow(`UPDATE stylist_presets SET name = ?, updated_at = datetime('now') WHERE id = ? RETURNING id`, name, id))
}

func (d *DB) DeleteStylistPreset(id int64) error {
	return stylistPresetChanged(id, d.db.QueryRow(`DELETE FROM stylist_presets WHERE id = ? RETURNING id`, id))
}

func stylistPresetChanged(id int64, row *sql.Row) error {
	var got int64
	if err := row.Scan(&got); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("заготовка стилиста %d не найдена", id)
		}
		return err
	}
	return nil
}
