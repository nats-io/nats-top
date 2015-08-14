package toputils

import (
	"github.com/nats-io/gnatsd/server"
)

type Stats struct {
	Varz  *server.Varz
	Connz *server.Connz
	Rates *Rates
}

type Rates struct {
	InMsgsRate   float64
	OutMsgsRate  float64
	InBytesRate  float64
	OutBytesRate float64
}

const (
	SortByCid      server.SortOpt = "cid"
	SortBySubs                    = "subs"
	SortByOutMsgs                 = "msgs_to"
	SortByInMsgs                  = "msgs_from"
	SortByOutBytes                = "bytes_to"
	SortByInBytes                 = "bytes_from"
)

type ByCid []*server.ConnInfo

func (d ByCid) Len() int {
	return len(d)
}
func (d ByCid) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByCid) Less(i, j int) bool {
	return d[i].Cid < d[j].Cid
}

type BySubs []*server.ConnInfo

func (d BySubs) Len() int {
	return len(d)
}
func (d BySubs) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d BySubs) Less(i, j int) bool {
	return d[i].NumSubs < d[j].NumSubs
}

type ByPending []*server.ConnInfo

func (d ByPending) Len() int {
	return len(d)
}
func (d ByPending) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByPending) Less(i, j int) bool {
	return d[i].Pending < d[j].Pending
}

type ByMsgsTo []*server.ConnInfo

func (d ByMsgsTo) Len() int {
	return len(d)
}
func (d ByMsgsTo) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByMsgsTo) Less(i, j int) bool {
	return d[i].OutMsgs < d[j].OutMsgs
}

type ByMsgsFrom []*server.ConnInfo

func (d ByMsgsFrom) Len() int {
	return len(d)
}
func (d ByMsgsFrom) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByMsgsFrom) Less(i, j int) bool {
	return d[i].InMsgs < d[j].InMsgs
}

type ByBytesTo []*server.ConnInfo

func (d ByBytesTo) Len() int {
	return len(d)
}
func (d ByBytesTo) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByBytesTo) Less(i, j int) bool {
	return d[i].OutBytes < d[j].OutBytes
}

type ByBytesFrom []*server.ConnInfo

func (d ByBytesFrom) Len() int {
	return len(d)
}
func (d ByBytesFrom) Swap(i, j int) {
	d[i], d[j] = d[j], d[i]
}
func (d ByBytesFrom) Less(i, j int) bool {
	return d[i].InBytes < d[j].InBytes
}
