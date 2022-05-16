package readcache

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/beego/beego/v2/client/cache"
	"github.com/rs/zerolog/log"
)

const defaultTimeout = 5 * time.Hour

var (
	redisCache cache.Cache
	memCache   cache.Cache
)

// 锁
var mutex sync.Mutex

// MustInit 初始化缓存(内存缓存+Redis缓存)
// redisIp: redis 服务的 IP
// redisPort: redis 服务的 Port
// redisDbNum: 使用 redis 服务的哪个 DB
// memoryCacheCheckIntervalInSecond: 每隔多少秒检查一次内存缓存
func MustInit(redisIp string, redisPort, redisDbNum int, memoryCacheCheckIntervalInSecond int) {
	var err error
	redisCache, err = cache.NewCache("redis", fmt.Sprintf(`{"conn":"%s:%d","dbNum":"%d"}`, redisIp, redisPort, redisDbNum))
	if err != nil {
		panic(err)
	}

	memCache, err = cache.NewCache("memory", fmt.Sprintf(`{"interval":%d}`, memoryCacheCheckIntervalInSecond))
	if err != nil {
		panic(err)
	}
}

// ----- ----- ----- ----- ----- ----- -----
// ----- ----- ----- ----- ----- ----- -----

// Get 读取值
// 返回值 err 说明：
// 		err != nil 就说明 发生错误 或 没有读到值
// 		err == nil 就说明 一切正常 并且 成功读到值
func Get[V any](k string, loadDataFromDbFunc *func(k string) (V, error)) (V, error) {
	// STEP1: 先从内存缓存中读取值
	value, err := memCache.Get(nil, k)
	if err == nil && value != nil { // CASE1: 内存缓存中有就直接返回
		v, ok := value.(V)
		if !ok {
			return nil, err
		}

		log.Debug().Msgf("get key[%s] from memory cache success")
		return v, nil
	} else { // STEP2: 内存缓存中没有再从二级缓存中读取
		value, err = redisCache.Get(nil, k)
		if err == nil && value != nil { // CASE2: 二级缓存中读取到值就返回它并将它写回内存缓存
			var v V
			err = unmarshal([]byte(fmt.Sprintf("%s", value)), &v)
			if err != nil {
				return nil, err
			}

			// 二级中有就需要写会内存缓存
			err = memCache.Put(nil, k, v, time.Second)
			if err != nil {
				log.Error().Msgf("get key[%s] from redis cache success,but write back to memory cache error: %s", k, err.Error())
			}

			log.Debug().Msgf("get key[%s] from redis cache success")
			return v, nil
		} else { // STEP3+CASE3: 内存缓存和二级缓存中都没有就从 db 读取数据并写回 cache
			if loadDataFromDbFunc == nil {
				return nil, fmt.Errorf("key[%s] not exist in cache", k)
			} else {
				log.Debug().Msgf("key[%s] not exist in cache, now reading from database and then write back to cache", k)

				// 加锁
				mutex.Lock()
				// 释放锁
				defer mutex.Unlock()

				// TODO 可以这样写吗？
				// 拿到锁后先尝试从内存缓存读取
				// 如果读到值了就说明已经从 db 中读到数据并写回缓存了
				// 如果没有读到值就需要去读 db 并写回缓存
				// <------ 因为当缓存中没有值，又有很多并发访问时可能回重复从 db 中加载数据，此处就是为了避免重复从 db 加载数据
				if value, err = memCache.Get(nil, k); err == nil && value != nil {
					v, ok := value.(V)
					if !ok {
						return nil, err
					}

					log.Debug().Msgf("get key[%s] from memory cache directly success when got mutex lock, so doesn't need to read db")
					return v, nil
					// ! 这段代码和上面的重复了 !
				}
				// TODO 可以这样写吗？

				// 从 db 中读取
				v, err := (*loadDataFromDbFunc)(k)
				if err != nil {
					return nil, err
				}

				// 写回 cache
				err = Put[V](k, v, defaultTimeout)
				if err != nil {
					log.Error().Msgf("put key[%s] to cache failed: %s", k, err.Error())
				}

				log.Debug().Msgf("read key[%s] from database success and write back to cache end", k)
				return v, nil
			}
		}
	}
}

// Put 设置值
// TODO 不要范型也可以吗？？
func Put[V any](k string, v V, timeout time.Duration) error {
	err := memCache.Put(nil, k, v, timeout)
	if err != nil {
		return err
	}

	value, err := marshal(v)
	if err != nil {
		return err
	}
	err = redisCache.Put(nil, k, string(value), timeout)
	if err != nil {
		return err
	}

	log.Debug().Msgf("put key[%s] to cache success", k)
	return nil
}

// Delete 删除值
func Delete(k string) error {
	err := memCache.Delete(nil, k)
	if err != nil {
		return err
	}

	err = redisCache.Delete(nil, k)
	if err != nil {
		return err
	}

	log.Debug().Msgf("delete key[%s] from cache success", k)
	return nil
}

func IsExist(k string) (bool, error) {
	exist, err := memCache.IsExist(nil, k)
	if err != nil {
		return false, err
	}

	if exist {
		return true, nil
	}

	return redisCache.IsExist(nil, k)
}

// ----- ----- ----- ----- ----- ----- -----
// ----- ----- ----- ----- ----- ----- -----

// encode data for saving it to kv cache
func marshal(v interface{}) ([]byte, error) {
	// json: func Marshal(v interface{}) ([]byte, error)
	// pb:   func Marshal(m Message) ([]byte, error)
	return json.Marshal(v)
}

// decode data for reading from kv cache to actual variable
func unmarshal(data []byte, v interface{}) error {
	// json: func Unmarshal(data []byte, v interface{}) error
	// pb:   func Unmarshal(b []byte, m Message) error
	return json.Unmarshal(data, v)
}
