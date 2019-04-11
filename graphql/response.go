package graphql

import (
	"encoding/json"
	"sync"
)

type syncResponse struct {
	Data   *syncMap
	Errors *syncMap
}

func (r *syncResponse) Load(key string) (interface{}, bool) {
	return r.Data.Load(key)
}

func (r *syncResponse) Delete(key string) {
	r.Data.Delete(key)
}

func (r *syncResponse) Store(key string, val interface{}) {
	r.Data.Store(key, val)
}

func (r *syncResponse) LoadErr(key string) (interface{}, bool) {
	return r.Errors.Load(key)
}

func (r *syncResponse) DeleteErr(key string) {
	r.Errors.Delete(key)
}

func (r *syncResponse) StoreErr(key string, val interface{}) {
	r.Errors.Store(key, val)
}

type syncMap struct {
	sync.RWMutex
	internal map[string]interface{}
}

func NewSyncResponse() *syncResponse {
	return &syncResponse{
		Data:   NewSyncMap(),
		Errors: NewSyncMap(),
	}
}

func NewSyncMap() *syncMap {
	return &syncMap{
		internal: make(map[string]interface{}),
	}
}

func (m *syncMap) Load(key string) (value interface{}, ok bool) {
	m.RLock()
	defer m.Unlock()
	result, ok := m.internal[key]
	return result, ok
}

func (m *syncMap) Delete(key string) {
	m.Lock()
	defer m.Unlock()
	delete(m.internal, key)
}

func (m *syncMap) Store(key string, value interface{}) {
	m.Lock()
	defer m.Unlock()
	// FIXME: Temporary hack to remove extra path info on error responses
	// Also for fixes json marshaling problems
	if e, ok := value.(*pathError); ok {
		e.path = e.path[1:len(e.path)]
		m.internal[key] = e.Error()
		return
	}
	m.internal[key] = value
}

func (m *syncMap) String() string {
	m.Lock()
	defer m.Unlock()

	result, err := json.Marshal(m.internal)
	if err != nil {
		return err.Error()
	}
	return string(result)
}

func (m syncMap) Error() string {
	return m.String()
}

func (m syncMap) Errors() []interface{} {
	m.Lock()
	defer m.Unlock()

	errors := []interface{}{}
	for _, v := range m.internal {
		errors = append(errors, v.(string))
	}
	return errors
}
