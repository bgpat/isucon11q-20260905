package main

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type isuMeta struct {
	ID         int    `db:"id"`
	JIAIsuUUID string `db:"jia_isu_uuid"`
	Name       string `db:"name"`
	Character  string `db:"character"`
	JIAUserID  string `db:"jia_user_id"`
}

type latestConditionRow struct {
	JIAIsuUUID string         `db:"jia_isu_uuid"`
	Timestamp  *time.Time     `db:"timestamp"`
	IsSitting  sql.NullBool   `db:"is_sitting"`
	Condition  sql.NullString `db:"condition"`
	Message    sql.NullString `db:"message"`
}

type isuMetaStore struct {
	sync.RWMutex
	byUUID      map[string]isuMeta
	usersLoaded map[string]struct{}
}

var isuMetas isuMetaStore

const isuMetaSelectColumns = "`id`, `jia_isu_uuid`, `name`, COALESCE(`character`, '') AS `character`, `jia_user_id`"

func (s *isuMetaStore) ensureLocked() {
	if s.byUUID == nil {
		s.byUUID = make(map[string]isuMeta)
	}
	if s.usersLoaded == nil {
		s.usersLoaded = make(map[string]struct{})
	}
}

func (s *isuMetaStore) get(jiaIsuUUID string) (isuMeta, bool) {
	s.RLock()
	defer s.RUnlock()
	m, ok := s.byUUID[jiaIsuUUID]
	return m, ok
}

func (s *isuMetaStore) list(jiaUserID string) ([]isuMeta, bool) {
	s.RLock()
	defer s.RUnlock()
	if _, ok := s.usersLoaded[jiaUserID]; !ok {
		return nil, false
	}
	out := make([]isuMeta, 0)
	for _, m := range s.byUUID {
		if m.JIAUserID == jiaUserID {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, true
}

func (s *isuMetaStore) store(m isuMeta) {
	s.Lock()
	s.ensureLocked()
	s.byUUID[m.JIAIsuUUID] = m
	s.Unlock()
	rememberIsuUUID(m.JIAIsuUUID)
}

func (s *isuMetaStore) storeUser(jiaUserID string, rows []isuMeta) {
	s.Lock()
	s.ensureLocked()
	for _, m := range rows {
		s.byUUID[m.JIAIsuUUID] = m
	}
	s.usersLoaded[jiaUserID] = struct{}{}
	s.Unlock()
	for _, m := range rows {
		rememberIsuUUID(m.JIAIsuUUID)
	}
}

func invalidateIsuMetaCache() {
	isuMetas.Lock()
	isuMetas.byUUID = nil
	isuMetas.usersLoaded = nil
	isuMetas.Unlock()
}

func rememberIsuMeta(m isuMeta) {
	isuMetas.store(m)
}

func listIsuMetaByUser(jiaUserID string) ([]isuMeta, error) {
	if metas, ok := isuMetas.list(jiaUserID); ok {
		return metas, nil
	}
	var rows []isuMeta
	err := db.Select(&rows, "SELECT "+isuMetaSelectColumns+" FROM `isu` WHERE `jia_user_id` = ?", jiaUserID)
	if err != nil {
		return nil, err
	}
	isuMetas.storeUser(jiaUserID, rows)
	metas, _ := isuMetas.list(jiaUserID)
	return metas, nil
}

func loadIsuMetaByUUID(jiaIsuUUID string) (isuMeta, bool, error) {
	if m, ok := isuMetas.get(jiaIsuUUID); ok {
		return m, true, nil
	}
	var m isuMeta
	err := db.Get(&m, "SELECT "+isuMetaSelectColumns+" FROM `isu` WHERE `jia_isu_uuid` = ?", jiaIsuUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return isuMeta{}, false, nil
	}
	if err != nil {
		return isuMeta{}, false, err
	}
	rememberIsuMeta(m)
	return m, true, nil
}

func ownedIsuMeta(jiaIsuUUID, jiaUserID string) (isuMeta, bool, error) {
	m, ok, err := loadIsuMetaByUUID(jiaIsuUUID)
	if err != nil || !ok || m.JIAUserID != jiaUserID {
		return isuMeta{}, false, err
	}
	return m, true, nil
}

func latestConditionsByUUIDs(uuids []string) ([]latestConditionRow, error) {
	if len(uuids) == 0 {
		return nil, nil
	}
	var b strings.Builder
	b.WriteString("SELECT u.jia_isu_uuid, latest.timestamp, latest.is_sitting, latest.condition, latest.message FROM (")
	args := make([]interface{}, 0, len(uuids))
	for i, uuid := range uuids {
		if i > 0 {
			b.WriteString(" UNION ALL ")
		}
		b.WriteString("SELECT ? AS jia_isu_uuid")
		args = append(args, uuid)
	}
	b.WriteString(") AS u LEFT JOIN isu_condition AS latest ON latest.id = (")
	b.WriteString("SELECT latest_condition.id FROM isu_condition AS latest_condition ")
	b.WriteString("WHERE latest_condition.jia_isu_uuid = u.jia_isu_uuid ")
	b.WriteString("ORDER BY latest_condition.timestamp DESC, latest_condition.id DESC LIMIT 1)")
	var rows []latestConditionRow
	err := db.Select(&rows, b.String(), args...)
	return rows, err
}
