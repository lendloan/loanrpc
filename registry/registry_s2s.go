// Package proxy is a registry plugin for the micro proxy
package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"go-micro.dev/v4/cmd"
	"go-micro.dev/v4/logger"
	"go-micro.dev/v4/registry"
	"go-micro.dev/v4/util/addr"
)

type SysInfo struct {
	HostName   string   `json:"hostname,omitempty"`
	OS         string   `json:"os,omitempty"`
	CpuNum     int      `json:"cpu_num,omitempty"`
	CpuPercent float64  `json:"cpu_percent,omitempty"`
	MemTotal   uint64   `json:"mem_total,omitempty"`
	MemUsed    uint64   `json:"mem_used,omitempty"`
	DiskTotal  uint64   `json:"disk_total,omitempty"`
	DiskUsed   uint64   `json:"disk_used,omitempty"`
	Ips        []string `json:"ips,omitempty"`
}

func getsysinfo() string {
	var sysinfo SysInfo

	hostinfo, err := host.Info()
	if nil == err {
		sysinfo.HostName = hostinfo.Hostname
		sysinfo.OS = hostinfo.OS
	}

	count, err := cpu.Counts(true)
	if nil == err {
		sysinfo.CpuNum = count
	}

	percent, err := cpu.Percent(time.Second, false)
	if nil == err && 0 < len(percent) {
		sysinfo.CpuPercent = percent[0]
	}

	memInfo, err := mem.VirtualMemory()
	if nil == err && nil != memInfo {
		sysinfo.MemTotal = memInfo.Total
		sysinfo.MemUsed = memInfo.Used
	}

	parts, err := disk.Partitions(true)
	if nil == err && 0 < len(parts) {
		for _, v := range parts {
			diskInfo, err := disk.Usage(v.Mountpoint)
			if nil != err {
				continue
			}

			sysinfo.DiskTotal = sysinfo.DiskTotal + diskInfo.Total
			sysinfo.DiskUsed = sysinfo.DiskUsed + diskInfo.Used
		}

	}

	sysinfo.Ips = addr.IPs()
	info, _ := json.Marshal(sysinfo)
	return string(info)
}

type proxy struct {
	opts registry.Options

	rwlock sync.RWMutex
	svrs   map[string][]*registry.Service
}

var watchNode []string

func init() {
	watchNode = make([]string, 0)
	cmd.DefaultRegistries["proxy"] = NewRegistry
}

// 设置要监听的节点连接信息
//
// @param nodes 配置中的nodes项
//
func SetWatchNode(nodes []string) {
	if 0 == len(nodes) {
		return
	}

	for _, v := range nodes {
		watchNode = append(watchNode, v)
	}

	return
}

func configure(s *proxy, opts ...registry.Option) error {
	for _, o := range opts {
		o(&s.opts)
	}
	var addrs []string
	for _, addr := range s.opts.Addrs {
		if len(addr) > 0 {
			addrs = append(addrs, addr)
		}
	}
	if len(addrs) == 0 {
		addrs = []string{"localhost:8081"}
	}
	registry.Addrs(addrs...)(&s.opts)
	return nil
}

var gs *proxy

func newRegistry(opts ...registry.Option) registry.Registry {
	if nil == gs {
		gs = &proxy{
			opts:   registry.Options{},
			rwlock: sync.RWMutex{},
			svrs:   make(map[string][]*registry.Service),
		}

		go gs.refresh()
		configure(gs, opts...)
	}

	return gs
}

func (s *proxy) Init(opts ...registry.Option) error {
	return configure(s, opts...)
}

func (s *proxy) Options() registry.Options {
	return s.opts
}

func (s *proxy) Register(service *registry.Service, opts ...registry.RegisterOption) error {
	if nil == service {
		err := errors.New("service is nil")

		return err
	}

	if GetDeregister().IsDe() {
		return nil
	}

	if nil == service.Metadata {
		service.Metadata = make(map[string]string)
	}
	service.Metadata["sysinfo"] = getsysinfo()

	b, err := json.Marshal(service)
	if err != nil {
		return err
	}

	var gerr error
	for _, addr := range s.opts.Addrs {
		scheme := "http"
		if s.opts.Secure {
			scheme = "https"
		}
		url := fmt.Sprintf("%s://%s/registry", scheme, addr)
		rsp, err := http.Post(url, "application/json", bytes.NewReader(b))
		if err != nil {
			gerr = err
			continue
		}
		if rsp.StatusCode != 200 {
			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				return err
			}
			rsp.Body.Close()
			gerr = errors.New(string(b))
			continue
		}
		io.Copy(ioutil.Discard, rsp.Body)
		rsp.Body.Close()

		GetDeregister().LocalSvr = service
		return nil
	}

	return gerr
}

func (s *proxy) Deregister(service *registry.Service, opts ...registry.DeregisterOption) error {
	b, err := json.Marshal(service)
	if err != nil {
		return err
	}

	var gerr error
	for _, addr := range s.opts.Addrs {
		scheme := "http"
		if s.opts.Secure {
			scheme = "https"
		}
		url := fmt.Sprintf("%s://%s/registry", scheme, addr)

		req, err := http.NewRequest("DELETE", url, bytes.NewReader(b))
		if err != nil {
			gerr = err
			continue
		}

		rsp, err := http.DefaultClient.Do(req)
		if err != nil {
			gerr = err
			continue
		}

		if rsp.StatusCode != 200 {
			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				return err
			}
			rsp.Body.Close()
			gerr = errors.New(string(b))
			continue
		}

		io.Copy(ioutil.Discard, rsp.Body)
		rsp.Body.Close()
		GetDeregister().De()

		return nil
	}
	return gerr
}

// 优先读取内存中的服务信息
//
// @param service 	服务名
// @return {[]Service,error}
//
func (s *proxy) GetService(service string, opts ...registry.GetOption) ([]*registry.Service, error) {
	if 0 == len(service) {
		logger.Info("service", service)

		return nil, errors.New("Service name is nil")
	}

	s.rwlock.RLock()
	defer s.rwlock.RUnlock()

	if _, ok := s.svrs[service]; ok {
		item := make([]*registry.Service, 0)
		for _, v := range s.svrs[service] {
			var svr registry.Service

			svr = *v
			item = append(item, &svr)
		}

		return item, nil
	}

	logger.Info(service, " node cache not exists.")
	return s.getService(service)
}

func (s *proxy) ListServices(opts ...registry.ListOption) ([]*registry.Service, error) {
	var gerr error
	logger.Info("ListServices")

	return nil, gerr
}

func (s *proxy) Watch(opts ...registry.WatchOption) (registry.Watcher, error) {
	var wo registry.WatchOptions
	for _, o := range opts {
		o(&wo)
	}
	logger.Info("Watch, Service: ", wo.Service)

	return newWatcher("")
}

func (s *proxy) String() string {
	return "proxy"
}

// 根据服务名获取服务列表
//
// @param service 	服务名
// @return {[]Service}
//
func (s *proxy) getService(service string) ([]*registry.Service, error) {
	if 0 == len(service) {
		return nil, errors.New("Service name is nil")
	}

	var gerr error
	for _, addr := range s.opts.Addrs {
		scheme := "http"
		if s.opts.Secure {
			scheme = "https"
		}

		url := fmt.Sprintf("%s://%s/registry/%s", scheme, addr, url.QueryEscape(service))
		rsp, err := http.Get(url)
		if err != nil {
			gerr = err
			continue
		}

		if rsp.StatusCode != 200 {
			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				return nil, err
			}
			rsp.Body.Close()
			gerr = errors.New(string(b))
			continue
		}

		b, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			gerr = err
			continue
		}
		rsp.Body.Close()
		var services []*registry.Service
		if err := json.Unmarshal(b, &services); err != nil {
			gerr = err
			continue
		}

		return services, nil
	}

	return nil, gerr
}

// 根据服务名批量获取服务列表
//
// @param s2sname 	服务名
// @return {[]Service}
//
func (s *proxy) getServices(s2sname string) (map[string][]*registry.Service, error) {
	if 0 == len(s2sname) {
		return nil, errors.New("Service name is nil")
	}

	var gerr error
	for _, addr := range s.opts.Addrs {
		scheme := "http"
		if s.opts.Secure {
			scheme = "https"
		}

		url := fmt.Sprintf("%s://%s/registry/%s", scheme, addr, url.QueryEscape(s2sname))
		rsp, err := http.Get(url)
		if err != nil {
			gerr = err
			continue
		}

		if rsp.StatusCode != 200 {
			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				return nil, err
			}
			rsp.Body.Close()
			gerr = errors.New(string(b))
			continue
		}

		b, err := ioutil.ReadAll(rsp.Body)
		if err != nil {
			gerr = err
			continue
		}
		rsp.Body.Close()
		var services map[string][]*registry.Service
		services = make(map[string][]*registry.Service)

		svrs := strings.Split(s2sname, ",")
		if 1 == len(svrs) {
			var serv []*registry.Service
			if err := json.Unmarshal(b, &serv); err != nil {
				gerr = err
				continue
			}

			services[s2sname] = serv
		} else {
			if err := json.Unmarshal(b, &services); err != nil {
				gerr = err
				continue
			}
		}

		return services, nil
	}

	return nil, gerr
}

// 刷新订阅的s2s信息
//
func (s *proxy) refresh() {
	if 0 == len(watchNode) {
		return
	}

	// 10s定时刷新订阅的服务信息
	ticker := time.NewTicker(time.Second * 10)
	for {
		select {
		case <-ticker.C:
			names := strings.Join(watchNode, ",")
			svrs, err := s.getServices(names)
			if nil != err {
				logger.Error("Refresh getService err ", err)

				continue
			}
			for k, v := range svrs {
				if 0 == len(v) {
					continue
				}

				s.rwlock.Lock()
				s.svrs[k] = v
				s.rwlock.Unlock()
			}
		}
	}
}

func NewRegistry(opts ...registry.Option) registry.Registry {
	return newRegistry(opts...)
}
