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
func Get[V any, LPARAM any](k string, loadDataParam LPARAM, loadDataFunc *func(k string, loadParam LPARAM) (*V, error)) (*V, error) {
	// STEP1: 先从内存缓存中读取值
	value, err := memCache.Get(nil, k) // 如果 k 不存在不会返回 nil 而是返回 error：`ERROR-4002015, the key isn't exist`
	if err == nil {                    // CASE1: 内存缓存中有值就直接返回 (值可以是nil)
		v, ok := value.(*V) // value 是 nil 时也可以正常进行类型转换
		if !ok {
			return nil, err
		}

		log.Debug().Msgf("get key[%s] from memory cache success", k)
		return v, nil
	} else { // STEP2: 内存缓存中没有再从二级缓存中读取
		value, err = redisCache.Get(nil, k)
		if err == nil && value != nil { // CASE2: 二级缓存中读取到值就返回它并将它写回内存缓存 (值不允许是nil)
			var v V
			err = unmarshal([]byte(fmt.Sprintf("%s", value)), &v) // 转换 value 为 string 类型，因为存到 redis 中的值都是 string 类型，而且值不可能是 nil
			if err != nil {
				return nil, err
			}

			// 二级中有就需要写回内存缓存
			err = memCache.Put(nil, k, &v, time.Second)
			if err != nil {
				log.Error().Msgf("get key[%s] from redis cache success,but write back to memory cache error: %s", k, err.Error())
			}

			log.Debug().Msgf("get key[%s] from redis cache success", k)
			return &v, nil
		} else { // STEP3+CASE3: 内存缓存和二级缓存中都没有就从 db 读取数据并写回 cache
			if loadDataFunc == nil {
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
					v, ok := value.(*V)
					if !ok {
						return nil, err
					}

					log.Debug().Msgf("get key[%s] from memory cache directly success when got mutex lock, so doesn't need to read db", k)
					return v, nil
					// ! 这段代码和上面的重复了 !
				}
				// TODO 可以这样写吗？

				// 从 db 中读取
				v, err := (*loadDataFunc)(k, loadDataParam)
				if err != nil {
					return nil, err
				}

				// loadDataFromDbFunc() 返回的 v 有可能是 nil, 但只要 err 是 nil 就表示调用成功, 可以正常使用 v
				// Put() 方法内部会正确处理 v 是 nil 的情况；此时 Get() 方法也应该要正常返回值是 nil 的 v

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

// GetWithLoadNil 读取值，loadDataFunc 使用 nil，即是缓存中没有时就直接返回 nil，不再调用 loadDataFunc 从其他数据源读取数据
func GetWithLoadNil[V any](k string) (*V, error) {
	return Get[V, struct{}](k, struct{}{}, nil)
}

// Put 设置值
// 要设置的值是 nil 时只能写到内存缓存，不允许写到 redis 缓存
// TODO 不要范型也可以吗？？
func Put[V any](k string, v *V, timeout time.Duration) error {
	err := memCache.Put(nil, k, v, timeout) // (值可以是nil)
	if err != nil {
		return err
	}

	// v 是 nil 时不允许写到 redis 缓存
	if v != nil {
		value, err := marshal(*v)
		if err != nil {
			return err
		}
		err = redisCache.Put(nil, k, string(value), timeout) // (值不允许是nil)
		if err != nil {
			return err
		}
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
