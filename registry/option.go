package registry

import (
	"sync"

	"go-micro.dev/v4/registry"
)

type Deregister struct {
	LocalSvr *registry.Service

	isDe bool
	lock sync.Mutex
}

var gDeregister *Deregister

func GetDeregister() *Deregister {
	if nil == gDeregister {
		gDeregister = &Deregister{
			LocalSvr: nil,
			isDe:     false,
			lock:     sync.Mutex{},
		}
	}

	return gDeregister
}

// 注销调用，说明已经注销
//
func (this *Deregister) De() {
	this.lock.Lock()
	defer this.lock.Unlock()

	this.isDe = true
	return
}

// 检查是否已经注销，主要用于注册的时候判断
//
// @return bool
//
func (this *Deregister) IsDe() bool {
	this.lock.Lock()
	defer this.lock.Unlock()

	return this.isDe
}
