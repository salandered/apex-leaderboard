## Draft

### Data loss

AOF alongside RDB - we lose 1 sec of data, acceptable for a leaderboard service

### Losing disc

Solution via backups.

### No classic transactions

* Atomic: using Lua scripts, they are atomic
* Isolation: redis is serializable by design + lua scripts for multi key queries
* no rollback (a failed command leaves other commands applied) - a real problem. Carefully designed and validated lua scripts? trying to achieve that

### Replication is asynchronous

Probably acceptable for a leaderboard service. Risks can be minimized with `min-replicas-to-write` / `min-replicas-max-lag`. Trying to prove it will work (planned)

### RAM ceiling

* our data is small and some things can be stored archived. 100M events is ~15 GB
* ledger (event log) can be trimmed (set-score events act as a barrier)
* `noeviction` policy. `maxmemory` to 60% of available

### No secondary indexes

Maintained by hand index structures. It hurts, though

## Links

Official docs

* Replication:
  https://redis.io/docs/latest/operate/oss_and_stack/management/replication/
* Cluster spec, write safety section:
  https://redis.io/docs/latest/operate/oss_and_stack/reference/cluster-spec/
* Cluster, see "can lose writes":
  https://redis.io/docs/latest/operate/oss_and_stack/management/scaling/
* Transactions:
  https://redis.io/docs/latest/develop/using-commands/transactions/

Other

* Redis-Raft 2020: https://jepsen.io/analyses/redis-raft-1b3fbf6
* https://antirez.com/news/55
* BGSAVE :
  https://oneuptime.com/blog/post/2026-03-31-redis-how-redis-handles-bgsave-during-memory-pressure/view
