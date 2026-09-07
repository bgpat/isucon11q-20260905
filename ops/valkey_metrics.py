"""Valkey スナップショットの before/after 差分を集計する。"""
import math

# CONFIG RESETSTAT で 0 に戻るカウンタ。差分の代わりに after を区間値にする。
RESET_COUNTERS = (
    'total_connections_received',
    'total_commands_processed',
    'total_net_input_bytes',
    'total_net_output_bytes',
    'rejected_connections',
    'expired_keys',
    'evicted_keys',
    'keyspace_hits',
    'keyspace_misses',
)
# RESETSTAT しても残るカウンタ。常に after-before を使う。
KEEP_COUNTERS = (
    'aof_delayed_fsync',
    'aof_rewrites',
)
# 取得時点の値。差分にしない。
GAUGES = (
    'instantaneous_ops_per_sec',
    'instantaneous_input_kbps',
    'instantaneous_output_kbps',
    'used_memory',
    'used_memory_rss',
    'mem_fragmentation_ratio',
    'connected_clients',
    'blocked_clients',
    'aof_enabled',
    'aof_current_size',
    'aof_base_size',
)
INFO_KEYS = RESET_COUNTERS + KEEP_COUNTERS + GAUGES


def _number(value):
    if value is None or value == '':
        return None
    if '.' in value:
        number = float(value)
        if not math.isfinite(number):
            raise ValueError('非有限値: ' + value)
        return number
    return int(value)


def _percentile(values, percent):
    if not values:
        return None
    ordered = sorted(values)
    if len(ordered) == 1:
        return ordered[0]
    rank = (len(ordered) - 1) * percent / 100
    low = int(rank)
    high = min(low + 1, len(ordered) - 1)
    frac = rank - low
    return ordered[low] * (1 - frac) + ordered[high] * frac


def parse_snapshot(text):
    info = {}
    commands = {}
    zcards = []
    latest_keys = None
    slowlog_len = None
    in_zcard = False
    for raw in text.splitlines():
        line = raw.rstrip('\r')
        if line.startswith('# slowlog_len='):
            slowlog_len = int(line.split('=', 1)[1])
            continue
        if line.startswith('# latest_keys='):
            latest_keys = int(line.split('=', 1)[1])
            continue
        if line == '=== ZCARD cond:z:* ===':
            in_zcard = True
            continue
        if line.startswith('=== ') and line.endswith(' ==='):
            in_zcard = False
            continue
        if in_zcard:
            if not line or line.startswith('#'):
                continue
            parts = line.split()
            if len(parts) != 2:
                raise ValueError('ZCARD行が不正: ' + line)
            zcards.append(int(parts[1]))
            continue
        if line.startswith('cmdstat_'):
            name, _, rest = line.partition(':')
            fields = dict(item.split('=', 1) for item in rest.split(',') if '=' in item)
            commands[name[len('cmdstat_'):]] = dict(
                calls=int(fields['calls']),
                usec=int(fields['usec']),
                rejected=int(fields.get('rejected_calls', '0')),
                failed=int(fields.get('failed_calls', '0')),
            )
            continue
        if ':' not in line or line.startswith('#') or line.startswith('==='):
            continue
        key, _, value = line.partition(':')
        if key in INFO_KEYS:
            info[key] = value
    return dict(info=info, commands=commands, zcards=zcards, latest_keys=latest_keys, slowlog_len=slowlog_len)


def _commands(before, after, reset):
    rows = []
    for name in set(before) | set(after):
        old = before.get(name, dict(calls=0, usec=0, rejected=0, failed=0))
        new = after.get(name, dict(calls=0, usec=0, rejected=0, failed=0))
        if reset:
            calls, usec, rejected, failed = new['calls'], new['usec'], new['rejected'], new['failed']
        else:
            calls = new['calls'] - old['calls']
            usec = new['usec'] - old['usec']
            rejected = new['rejected'] - old['rejected']
            failed = new['failed'] - old['failed']
        if calls < 0 or usec < 0:
            raise ValueError('commandstats が減少している: ' + name)
        if calls == 0 and usec == 0 and rejected == 0 and failed == 0:
            continue
        per = usec / calls if calls else None
        rows.append(dict(command=name, calls=calls, usec=usec, usec_per_call=per, rejected=rejected, failed=failed))
    rows.sort(key=lambda row: (-row['usec'], -row['calls'], row['command']))
    return rows


def _stats(before_info, after_info, reset):
    rows = []
    for key in INFO_KEYS:
        old = _number(before_info.get(key))
        new = _number(after_info.get(key))
        if new is None and old is None:
            continue
        if key in GAUGES:
            delta = None if old is None or new is None else new - old
            rows.append(dict(key=key, before=old, after=new, delta=delta, kind='gauge'))
            continue
        if key in RESET_COUNTERS and reset:
            rows.append(dict(key=key, before=0, after=new, delta=new, kind='interval'))
            continue
        delta = None if old is None or new is None else new - old
        if delta is not None and delta < 0:
            raise ValueError('カウンタが減少している: ' + key)
        rows.append(dict(key=key, before=old, after=new, delta=delta, kind='counter'))
    return rows


def analyze(before_text, after_text):
    if not after_text.strip():
        raise ValueError('valkey-after が空')
    after = parse_snapshot(after_text)
    if 'total_commands_processed' not in after['info'] and not after['commands']:
        raise ValueError('valkey-after に INFO commandstats が見つからない')
    before = parse_snapshot(before_text) if before_text.strip() else dict(
        info={}, commands={}, zcards=[], latest_keys=None, slowlog_len=None)
    before_total = _number(before['info'].get('total_commands_processed')) or 0
    after_total = _number(after['info'].get('total_commands_processed')) or 0
    reset = after_total < before_total
    if not reset:
        for name, old in before['commands'].items():
            new = after['commands'].get(name)
            if new and (new['calls'] < old['calls'] or new['usec'] < old['usec']):
                reset = True
                break
    cards = after['zcards']
    return dict(
        reset=reset,
        slowlog_len=after['slowlog_len'],
        commands=_commands(before['commands'], after['commands'], reset),
        stats=_stats(before['info'], after['info'], reset),
        zset=dict(
            keys=len(cards),
            members=sum(cards) if cards else 0,
            max=max(cards) if cards else 0,
            p50=_percentile(cards, 50),
            p95=_percentile(cards, 95),
            latest_keys=after['latest_keys'],
        ),
    )
