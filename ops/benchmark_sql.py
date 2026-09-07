"""全slow logをLOADの整数秒窓に限定して集計する。"""
import collections
import gzip
import re


def analyze(path, start, end):
    groups = collections.defaultdict(lambda: dict(count=0, seconds=0., max_seconds=0., lock=0., examined=0, sent=0))
    current = None

    def finish():
        if not current or not start <= current['timestamp'] < end:
            return
        sql = ' '.join(current['sql'])
        if sql.startswith('INSERT INTO `isu`'):
            sql = 'INSERT INTO `isu` (画像含む) VALUES (?)'
        sql = re.sub(r"'(?:[^'\\]|\\.|'')*'", '?', sql)
        sql = re.sub(r'"(?:[^"\\]|\\.)*"', '?', sql)
        sql = re.sub(r'\b\d+(?:\.\d+)?\b', '?', sql)
        sql = re.sub(r'\s+', ' ', sql).strip()
        sql = re.sub(r'(\([^()]+\))(?:,\s*\1)+', r'\1 [repeated]', sql)
        group = groups[sql]
        group['count'] += 1
        group['max_seconds'] = max(group['max_seconds'], current['seconds'])
        for key in ('seconds', 'lock', 'examined', 'sent'):
            group[key] += current[key]

    with gzip.open(path, 'rt', errors='replace') as source:
        for line in source:
            if line.startswith('# Query_time:'):
                finish()
                values = re.search(r'Query_time: ([\d.]+)\s+Lock_time: ([\d.]+)\s+Rows_sent: (\d+)\s+Rows_examined: (\d+)', line)
                if not values:
                    raise ValueError('slow log Query_time形式が不正')
                current = dict(seconds=float(values[1]), lock=float(values[2]), sent=int(values[3]), examined=int(values[4]), timestamp=0, sql=[])
            elif current and line.startswith('SET timestamp='):
                current['timestamp'] = int(re.search(r'=(\d+)', line)[1])
            elif current and (not line.startswith('#') or line.startswith('# administrator command:')):
                current['sql'].append(line.strip())
        finish()
    return [dict(sql=sql, **value) for sql, value in sorted(groups.items(), key=lambda item: -item[1]['seconds'])]
