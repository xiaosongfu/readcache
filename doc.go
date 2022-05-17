package readcache

// Put Logic:
// value 不管是 nil 还是有值都会被保存到 memory cache 中；
// value 为 nil 时不会保存到 redis cache 中，只有当 value 不为 nil 时才会经编码并转换为 string 值后保存到 redis cache 中

// Get Logic:
// 先读取 memory cache，如果没有发生错误，即使值为 nil，也算作从 memory cache 中读取值成功，直接返回即可 (如果 key 不存在不会返回 nil 而是返回 error：`ERROR-4002015, the key isn't exist`)
// 如果读取 memory cache 发生错误则开始读取 redis cache，如果没有发生错误并且值不是 nil，才算作从 redis cache 中读取成功，直接返回即可(需要从 string 值解码为真实的类型)
// 如果读取 redis cache 发生错误或值是 nil 就需要从 db 中读取值，从 db 中读取值时如果发生错误就直接返回错误，如果没有发生错误，即使读取的值是 nil 也算作读取成功，正常写回缓存并正常返回即可
