package botex

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/emirpasic/gods/maps/treemap"
	"github.com/emirpasic/gods/utils"
	"github.com/scraperwall/botex/config"
	log "github.com/sirupsen/logrus"
)

type Stats struct {
	Total       int64     `json:"total"`
	Whitelisted int64     `json:"whitelisted"`
	Blocked     int64     `json:"blocked"`
	Human       int64     `json:"human"`
	Time        time.Time `json:"time,string"`
	UpdatedAt   time.Time `json:"updated_at,string"`
}

type StatsWindows struct {
	Stats
	Map       *treemap.Map `json:"data"`
	config    *config.Config
	resources *Resources
	mutex     sync.RWMutex
}

func NewStatsWindows(resources *Resources, config *config.Config) *StatsWindows {
	stats := StatsWindows{
		Map:       treemap.NewWith(utils.TimeComparator),
		Stats:     Stats{},
		config:    config,
		resources: resources,
		mutex:     sync.RWMutex{},
	}

	return &stats
}

func (s *StatsWindows) All() map[string]Stats {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	iter := s.Map.Iterator()

	res := make(map[string]Stats)

	for iter.Next() {
		key := iter.Key().(time.Time)
		stats := iter.Value().(Stats)

		res[key.Format(time.RFC3339Nano)] = stats
	}

	return res
}

func (s *StatsWindows) Add(d Decision, t time.Time) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	k := s.keyFor(t)
	var stats Stats

	statsRaw, ok := s.Map.Get(time.Time(k))
	if !ok {
		// store the previous Stats item
		if s.Map.Size() > 0 {
			latestKey, latestValue := s.Map.Max()
			latestBytes, err := json.Marshal(latestValue)
			if err != nil {
				log.Errorf("json encoding error of Stats: %s", err)
				return err
			}
			latestKeyBytes := []byte(fmt.Sprintf("stats:%d", latestKey.(time.Time).UnixNano()))
			s.resources.Store.SetEx(latestKeyBytes, latestBytes, time.Hour*24)
		}

		// create a new Stats item
		stats = Stats{
			Time: t.Truncate(s.config.WindowSize),
		}
	} else {
		stats = statsRaw.(Stats)
	}

	switch d {
	case IsBlocked:
		s.Blocked++
		stats.Blocked++
	case IsWhitelisted:
		s.Whitelisted++
		stats.Whitelisted++
	default:
		stats.Human++
		s.Human++
	}
	stats.Total++
	s.Total++

	stats.UpdatedAt = time.Now()

	// log.Infof("stats size: %d", binary.Size(stats))
	s.Map.Put(time.Time(k), stats)

	return nil
}

func (s *StatsWindows) Expire() {
	threshold := time.Now().Add(s.config.WindowSize * -time.Duration(s.config.NumWindows))
	s.mutex.Lock()
	defer s.mutex.Unlock()

	iter := s.Map.Iterator()

	for iter.Next() {
		key := iter.Key().(time.Time)
		if key.After(threshold) {
			break
		}

		stats := iter.Value().(Stats)
		s.Total -= stats.Total
		s.Blocked -= stats.Blocked
		s.Human -= stats.Human
		s.Whitelisted -= stats.Whitelisted

		s.Map.Remove(key)
	}
}

func (s *StatsWindows) keyFor(t time.Time) time.Time {
	return t.Truncate(s.config.WindowSize)
}

func (s Stats) MarshalJSON() ([]byte, error) {
	data := struct {
		Total       int64  `json:"total"`
		Whitelisted int64  `json:"whitelisted"`
		Blocked     int64  `json:"blocked"`
		Human       int64  `json:"human"`
		Time        string `json:"time"`
		UpdatedAt   string `json:"updated_at"`
	}{
		Total:       s.Total,
		Whitelisted: s.Whitelisted,
		Blocked:     s.Blocked,
		Human:       s.Human,
		Time:        s.Time.Format(time.RFC3339Nano),
		UpdatedAt:   s.Time.Format(time.RFC3339Nano),
	}

	return json.Marshal(data)
}

func (s *Stats) UnmarshalJSON(bytes []byte) error {
	data := struct {
		Total       int64  `json:"total"`
		Whitelisted int64  `json:"whitelisted"`
		Blocked     int64  `json:"blocked"`
		Human       int64  `json:"human"`
		Time        string `json:"time"`
		UpdatedAt   string `json:"updated_at"`
	}{}

	err := json.Unmarshal(bytes, &data)
	if err != nil {
		return err
	}

	s.Total = data.Total
	s.Whitelisted = data.Whitelisted
	s.Blocked = data.Blocked
	s.Human = data.Human

	t, err := time.Parse(time.RFC3339Nano, data.Time)
	if err != nil {
		return err
	}
	s.Time = t

	t, err = time.Parse(time.RFC3339Nano, data.UpdatedAt)
	if err != nil {
		return err
	}
	s.UpdatedAt = t

	return nil
}
