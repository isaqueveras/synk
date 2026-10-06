package synk

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics é um snapshot das métricas do producer.
type Metrics struct {
	ActiveJobs   int64   `json:"active_jobs"`
	ActiveJobIDs []JobID `json:"active_job_ids"`

	JobsFetched   int64 `json:"jobs_fetched"`
	JobsStarted   int64 `json:"jobs_started"`
	JobsCompleted int64 `json:"jobs_completed"`
	JobsFailed    int64 `json:"jobs_failed"`
	JobsCancelled int64 `json:"jobs_cancelled"`

	HeartbeatsSent  int64     `json:"heartbeats_sent"`
	HeartbeatErrors int64     `json:"heartbeat_errors"`
	LastHeartbeat   time.Time `json:"last_heartbeat"`

	LastJobStarted   time.Time `json:"last_job_started"`
	LastJobCompleted time.Time `json:"last_job_completed"`
}

type metricsState struct {
	activeJobs atomic.Int64

	activeJobIDs      map[JobID]struct{}
	activeJobIDsMutex sync.Mutex

	jobsFetched   atomic.Int64
	jobsStarted   atomic.Int64
	jobsCompleted atomic.Int64
	jobsFailed    atomic.Int64
	jobsCancelled atomic.Int64

	heartbeatsSent  atomic.Int64
	heartbeatErrors atomic.Int64

	lastHeartbeat    atomic.Int64
	lastJobStarted   atomic.Int64
	lastJobCompleted atomic.Int64
}

func (m *metricsState) snapshot() *Metrics {
	m.activeJobIDsMutex.Lock()
	activeJobIDs := make([]JobID, 0, len(m.activeJobIDs))
	for id := range m.activeJobIDs {
		activeJobIDs = append(activeJobIDs, id)
	}
	m.activeJobIDsMutex.Unlock()

	sort.Slice(activeJobIDs, func(i, j int) bool {
		return int64(activeJobIDs[i]) < int64(activeJobIDs[j])
	})

	parseTime := func(unixNano int64) time.Time {
		if unixNano == 0 {
			return time.Time{}
		}
		return time.Unix(0, unixNano)
	}

	return &Metrics{
		ActiveJobs:       m.activeJobs.Load(),
		ActiveJobIDs:     activeJobIDs,
		JobsFetched:      m.jobsFetched.Load(),
		JobsStarted:      m.jobsStarted.Load(),
		JobsCompleted:    m.jobsCompleted.Load(),
		JobsFailed:       m.jobsFailed.Load(),
		JobsCancelled:    m.jobsCancelled.Load(),
		HeartbeatsSent:   m.heartbeatsSent.Load(),
		HeartbeatErrors:  m.heartbeatErrors.Load(),
		LastHeartbeat:    parseTime(m.lastHeartbeat.Load()),
		LastJobStarted:   parseTime(m.lastJobStarted.Load()),
		LastJobCompleted: parseTime(m.lastJobCompleted.Load()),
	}
}

func (m *metricsState) addActiveJob(id JobID) {
	m.activeJobIDsMutex.Lock()
	defer m.activeJobIDsMutex.Unlock()

	if m.activeJobIDs == nil {
		m.activeJobIDs = make(map[JobID]struct{})
	}

	if _, exists := m.activeJobIDs[id]; !exists {
		m.activeJobIDs[id] = struct{}{}
		m.activeJobs.Add(1)
	}
}

func (m *metricsState) removeActiveJob(id JobID) {
	m.activeJobIDsMutex.Lock()
	defer m.activeJobIDsMutex.Unlock()

	if _, exists := m.activeJobIDs[id]; exists {
		delete(m.activeJobIDs, id)
		m.activeJobs.Add(-1)
	}
}
