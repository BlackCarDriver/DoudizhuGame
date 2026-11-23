package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// 积分存档：沿用梭哈模拟器的本地文本存档方式。
//   - 一个昵称一条记录，全局共享；
//   - 新人首次进入发放启动积分；余额低于上桌门槛时每天补一次救济金；
//   - 每天完成一局后发一次活跃奖励；
//   - 输赢在局末一次性写入，进程被 kill 也不会只写一半（临时文件 + rename）。
//
// 数据量很小（房间成员 + 历史玩家），不需要数据库。

const playersVersion = 1

type PlayerRecord struct {
	Nick             string `json:"nick"`
	Avatar           string `json:"avatar"`
	Points           int    `json:"points"`
	Wins             int    `json:"wins"`
	Losses           int    `json:"losses"`
	LastRelief       string `json:"lastRelief,omitempty"`
	LastActiveReward string `json:"lastActiveReward,omitempty"`
	LastGame         string `json:"lastGame,omitempty"`
	UpdatedAt        int64  `json:"updatedAt,omitempty"`
}

type playersData struct {
	Version int                      `json:"version"`
	Players map[string]*PlayerRecord `json:"players"`
}

type Store struct {
	mu                sync.Mutex
	path              string
	data              playersData
	initialPoints     int
	minPointsToSit    int
	dailyActiveReward int
}

func NewStore(path string, initialPoints, minPointsToSit, dailyActiveReward int) *Store {
	if path == "" {
		path = filepath.Join("data", "players.json")
	}
	if initialPoints < 0 {
		initialPoints = 0
	}
	if minPointsToSit < 0 {
		minPointsToSit = 0
	}
	if dailyActiveReward < 0 {
		dailyActiveReward = 0
	}
	return &Store{
		path:              path,
		data:              playersData{Version: playersVersion, Players: map[string]*PlayerRecord{}},
		initialPoints:     initialPoints,
		minPointsToSit:    minPointsToSit,
		dailyActiveReward: dailyActiveReward,
	}
}

func (s *Store) Load() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// 主档不存在时尝试 .bak
		if bak, berr := os.ReadFile(s.path + ".bak"); berr == nil {
			raw = bak
			log.Printf("[store] 主存档不存在，已从 %s.bak 恢复", s.path)
		} else {
			return nil
		}
	}
	if len(raw) == 0 {
		return nil
	}
	var d playersData
	if err := json.Unmarshal(raw, &d); err != nil {
		bad := s.path + ".bad"
		_ = os.Rename(s.path, bad)
		log.Printf("[store] 积分存档损坏（%v），已移到 %s", err, bad)
		// 尝试从 .bak 恢复
		if bak, berr := os.ReadFile(s.path + ".bak"); berr == nil {
			var bd playersData
			if json.Unmarshal(bak, &bd) == nil {
				d = bd
				log.Printf("[store] 已从 %s.bak 恢复积分存档", s.path)
			} else {
				log.Printf("[store] 备份也损坏（%v），从空档开始", err)
				return nil
			}
		} else {
			log.Printf("[store] 没有可用备份，从空档开始")
			return nil
		}
	}
	if d.Players == nil {
		d.Players = map[string]*PlayerRecord{}
	}
	d.Version = playersVersion
	s.mu.Lock()
	s.data = d
	s.mu.Unlock()
	log.Printf("[store] 已加载 %s（%d 个玩家）", s.path, len(d.Players))
	return nil
}

func today() string {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.Local
	}
	return time.Now().In(loc).Format("2006-01-02")
}

// Touch 玩家进入游戏：创建记录、更新头像、检查破产救济。返回当前积分。
func (s *Store) Touch(nick, avatar string) int {
	if nick == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.ensureLocked(nick, avatar)
	changed := false
	if avatar != "" && rec.Avatar != avatar {
		rec.Avatar = avatar
		changed = true
	}
	day := today()
	if s.grantReliefLocked(rec, day) {
		changed = true
	}
	if changed {
		rec.UpdatedAt = time.Now().Unix()
		s.writeLocked()
	}
	return rec.Points
}

// ClaimRelief 余额低于最低上桌积分时，每天补到最低上桌积分。
// 不要求玩家先上桌/先打一局，否则 0 分新人会死锁。
func (s *Store) ClaimRelief(nick string) (int, bool) {
	if nick == "" {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.ensureLocked(nick, "")
	day := today()
	before := rec.Points
	if !s.grantReliefLocked(rec, day) {
		return 0, false
	}
	rec.UpdatedAt = time.Now().Unix()
	s.writeLocked()
	return rec.Points - before, true
}

// ClaimActiveReward 当天完成过至少一局，且余额达到最低上桌积分时发放一次。
func (s *Store) ClaimActiveReward(nick string) (int, bool) {
	if nick == "" {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.ensureLocked(nick, "")
	day := today()
	if !s.grantActiveRewardLocked(rec, day) {
		return 0, false
	}
	rec.UpdatedAt = time.Now().Unix()
	s.writeLocked()
	return s.dailyActiveReward, true
}

func (s *Store) grantReliefLocked(rec *PlayerRecord, day string) bool {
	if s.minPointsToSit <= 0 || rec.Points >= s.minPointsToSit || rec.LastRelief == day {
		return false
	}
	before := rec.Points
	rec.Points = s.minPointsToSit
	rec.LastRelief = day
	log.Printf("[store] %s 领取破产救济 %d 分（%d → %d）", rec.Nick, rec.Points-before, before, rec.Points)
	return true
}

func (s *Store) grantActiveRewardLocked(rec *PlayerRecord, day string) bool {
	if s.dailyActiveReward <= 0 || rec.LastGame != day || rec.LastActiveReward == day {
		return false
	}
	if rec.Points < s.minPointsToSit {
		return false
	}
	rec.Points += s.dailyActiveReward
	rec.LastActiveReward = day
	log.Printf("[store] %s 领取每日活跃奖励 %d 分（当前 %d）", rec.Nick, s.dailyActiveReward, rec.Points)
	return true
}

// Points 查询积分（不发放赠送，不写盘）。
func (s *Store) Points(nick string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rec := s.data.Players[nick]; rec != nil {
		return rec.Points
	}
	return s.initialPoints
}

// ApplyResult 局末写入积分变化与胜负。
func (s *Store) ApplyResult(deltas map[string]int, win map[string]bool, avatars map[string]string) {
	if len(deltas) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for nick, delta := range deltas {
		if nick == "" {
			continue
		}
		rec := s.ensureLocked(nick, avatars[nick])
		if avatars[nick] != "" {
			rec.Avatar = avatars[nick]
		}
		rec.Points += delta
		if rec.Points < 0 {
			rec.Points = 0
		}
		if win[nick] {
			rec.Wins++
		} else {
			rec.Losses++
		}
		rec.LastGame = today()
		rec.UpdatedAt = time.Now().Unix()
		changed = true
	}
	if changed {
		s.writeLocked()
	}
}

func (s *Store) ensureLocked(nick, avatar string) *PlayerRecord {
	rec := s.data.Players[nick]
	if rec == nil {
		rec = &PlayerRecord{Nick: nick, Avatar: avatar, Points: s.initialPoints}
		s.data.Players[nick] = rec
	}
	return rec
}

// HasAvatar 头像代理白名单：历史玩家用过的头像。
func (s *Store) HasAvatar(raw string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range s.data.Players {
		if rec.Avatar == raw {
			return true
		}
	}
	return false
}

// RankRow 排行榜一行。
type RankRow struct {
	Rank   int    `json:"rank"`
	Nick   string `json:"nick"`
	Avatar string `json:"avatar"`
	Points int    `json:"points"`
	Wins   int    `json:"wins"`
	Losses int    `json:"losses"`
}

// Snapshot 排行榜：按积分降序，同分看胜场，再看负场少。
func (s *Store) Snapshot(limit int, me string) map[string]interface{} {
	if limit <= 0 {
		limit = 100
	}
	s.mu.Lock()
	rows := make([]RankRow, 0, len(s.data.Players))
	for _, rec := range s.data.Players {
		rows = append(rows, RankRow{
			Nick: rec.Nick, Avatar: rec.Avatar, Points: rec.Points,
			Wins: rec.Wins, Losses: rec.Losses,
		})
	}
	s.mu.Unlock()

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Points != rows[j].Points {
			return rows[i].Points > rows[j].Points
		}
		if rows[i].Wins != rows[j].Wins {
			return rows[i].Wins > rows[j].Wins
		}
		if rows[i].Losses != rows[j].Losses {
			return rows[i].Losses < rows[j].Losses
		}
		return rows[i].Nick < rows[j].Nick
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
	top := rows
	if len(top) > limit {
		top = top[:limit]
	}
	var my *RankRow
	for i := range rows {
		if rows[i].Nick == me {
			r := rows[i]
			my = &r
			break
		}
	}
	return map[string]interface{}{
		"rows":  top,
		"me":    my,
		"total": len(rows),
	}
}

func (s *Store) writeLocked() {
	s.data.Version = playersVersion
	if s.data.Players == nil {
		s.data.Players = map[string]*PlayerRecord{}
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		log.Printf("[store] 序列化失败: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		log.Printf("[store] 创建目录失败: %v", err)
		return
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		log.Printf("[store] 写临时文件失败: %v", err)
		return
	}
	if _, err := f.Write(raw); err != nil {
		_ = f.Close()
		log.Printf("[store] 写临时文件失败: %v", err)
		return
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		log.Printf("[store] 同步临时文件失败: %v", err)
		return
	}
	if err := f.Close(); err != nil {
		log.Printf("[store] 关闭临时文件失败: %v", err)
		return
	}
	// 保留上一份为 .bak：先复制旧档，再原子替换。
	if old, err := os.ReadFile(s.path); err == nil && len(old) > 0 {
		_ = os.WriteFile(s.path+".bak", old, 0o644)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		log.Printf("[store] 替换存档失败: %v", err)
	}
}
