package preset

import (
	"database/sql"
	"errors"
	"strings"
)

var (
	ErrStylistPlanExists   = errors.New("план с таким названием уже есть")
	ErrStylistPlanNotFound = errors.New("план не найден")
)

type StylistPlan struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Plan      string `json:"plan"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func StylistPlanName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

func stylistPlanKey(name string) string {
	return strings.ToLower(StylistPlanName(name))
}

func (d *DB) ListStylistPlans() ([]StylistPlan, error) {
	rows, err := d.db.Query(`SELECT id, name, plan, created_at, updated_at FROM stylist_plans ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StylistPlan{}
	for rows.Next() {
		var p StylistPlan
		if err := rows.Scan(&p.ID, &p.Name, &p.Plan, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) GetStylistPlan(id int64) (*StylistPlan, error) {
	var p StylistPlan
	err := d.db.QueryRow(`SELECT id, name, plan, created_at, updated_at FROM stylist_plans WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Plan, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrStylistPlanNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (d *DB) SaveStylistPlan(name, plan string, replace bool) (*StylistPlan, error) {
	name = StylistPlanName(name)
	key := stylistPlanKey(name)
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRow(`SELECT id FROM stylist_plans WHERE name_key = ?`, key).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		res, err := tx.Exec(`INSERT INTO stylist_plans (name, name_key, plan) VALUES (?, ?, ?)`, name, key, plan)
		if err != nil {
			return nil, err
		}
		if id, err = res.LastInsertId(); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	case !replace:
		return nil, ErrStylistPlanExists
	default:
		if _, err := tx.Exec(`UPDATE stylist_plans SET name = ?, plan = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%SZ','now') WHERE id = ?`, name, plan, id); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetStylistPlan(id)
}

func (d *DB) RenameStylistPlan(id int64, name string) (*StylistPlan, error) {
	name = StylistPlanName(name)
	key := stylistPlanKey(name)
	var other int64
	err := d.db.QueryRow(`SELECT id FROM stylist_plans WHERE name_key = ?`, key).Scan(&other)
	if err == nil && other != id {
		return nil, ErrStylistPlanExists
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	res, err := d.db.Exec(`UPDATE stylist_plans SET name = ?, name_key = ? WHERE id = ?`, name, key, id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrStylistPlanExists
		}
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrStylistPlanNotFound
	}
	return d.GetStylistPlan(id)
}

func (d *DB) DeleteStylistPlan(id int64) error {
	res, err := d.db.Exec(`DELETE FROM stylist_plans WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStylistPlanNotFound
	}
	return nil
}
