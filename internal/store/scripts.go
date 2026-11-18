package store

import "github.com/redis/go-redis/v9"

// createScript atomically creates a secret unless the id already exists as a
// secret or tombstone.
// KEYS[1]=cinder:s:{id} KEYS[2]=cinder:t:{id} ARGV[1]=value ARGV[2]=ttl_ms
var createScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 or redis.call('EXISTS', KEYS[2]) == 1 then
  return 0
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return 1
`)

// consumeScript atomically reads and burns a secret, leaving a tombstone with
// the remaining TTL.
var consumeScript = redis.NewScript(`
local pttl = redis.call('PTTL', KEYS[1])
if pttl <= 0 then
  if redis.call('EXISTS', KEYS[2]) == 1 then return {'consumed'} end
  return {'notfound'}
end
local v = redis.call('GET', KEYS[1])
redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[2], '1', 'PX', pttl)
return {'ok', v, pttl}
`)

// statusScript reads both keys' TTLs in one atomic step.
// Returns {state, pttl} where state is 1 available, 2 consumed, 0 gone.
var statusScript = redis.NewScript(`
local p = redis.call('PTTL', KEYS[1])
if p > 0 then return {1, p} end
local t = redis.call('PTTL', KEYS[2])
if t > 0 then return {2, t} end
return {0, 0}
`)
