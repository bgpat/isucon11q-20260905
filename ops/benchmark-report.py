#!/usr/bin/env python3
"""保存済み比較計測を集計・圧縮する。公開は --publish 指定時だけ行う。"""
import argparse
import concurrent.futures
import datetime
import json
import math
import pathlib
import re
import subprocess
import tarfile
import time

from benchmark_sql import analyze
from valkey_metrics import analyze as analyze_valkey

WRAPPER = pathlib.Path(__file__).with_name('benchmark-assets.sh')


def command(stage, args):
    return subprocess.check_output(['sh', str(WRAPPER), 'time', stage, '--', *args], text=True)


def profile(path, cumulative):
    raw = command('analysis-pprof-' + path.parent.name + ('-cum' if cumulative else '-flat'),
                  ['go', 'tool', 'pprof', '-top', '-nodecount=20', *(['-cum'] if cumulative else []), str(path)])
    rows = []
    for line in raw.splitlines():
        fields = line.split()
        if len(fields) >= 6 and fields[1].endswith('%'):
            rows.append([' '.join(fields[5:]), fields[0], fields[1], fields[3], fields[4]])
    metadata = [line for line in raw.splitlines() if line.startswith(('Time:', 'Duration:'))]
    if len(metadata) != 2:
        raise ValueError('profile時刻・期間・総sampleが見つからない')
    return dict(raw=raw, rows=rows, metadata=metadata)


def cpu(path):
    groups = {}
    for series in json.loads(path.read_text())['data']['result']:
        # 異なるホストの同名プロセスを混ぜない。
        key = series['metric'].get('instance', '') + '/' + series['metric'].get('groupname', '')
        values = groups.setdefault(key, {})
        for stamp, value in series['values']:
            number = float(value)
            if not math.isfinite(number):
                raise ValueError('CPU系列に非有限値: ' + key)
            values[float(stamp)] = values.get(float(stamp), 0) + number
    result = {}
    for key, values in groups.items():
        items = sorted(values.items())
        seconds = sum((b[0]-a[0])*(a[1]+b[1])/2 for a, b in zip(items, items[1:]))
        result[key] = dict(seconds=seconds, mean_cores=seconds/(items[-1][0]-items[0][0]) if len(items)>1 else None,
                           start=items[0][0] if items else None, end=items[-1][0] if items else None)
    return result


def final_score(log):
    load_end = log.rfind('<=== LOAD END')
    finish = log.rfind('<=== sendResult finish')
    if load_end < 0 or finish < load_end:
        raise ValueError('ベンチ未完走: LOAD END/sendResult finishが必要')
    scores = list(re.finditer(r'score:\s*(\d+)\((\d+)\s*-\s*(\d+)\)\s*:\s*(\w+)', log))
    if not scores:
        raise ValueError('最終scoreが見つからない')
    def stamp(position):
        line = log[log.rfind('\n',0,position)+1:position]
        matched = re.match(r'(\d\d):(\d\d):(\d\d(?:\.\d+)?)\s',line)
        if not matched:
            return None
        hour,minute,second = map(float,matched.groups())
        return hour*3600+minute*60+second
    end_time, finish_time = stamp(load_end), stamp(finish)
    selected = scores[-1]
    if end_time is not None and finish_time is not None:
        # stdout/stderr合流では物理行順が逆転する。24時跨ぎも秒差で扱う。
        duration = (finish_time-end_time) % 86400
        if duration > 60:
            raise ValueError('完走marker時刻が不整合')
        candidates = [( (stamp(s.start())-end_time) % 86400, s) for s in scores if stamp(s.start()) is not None]
        candidates = [(delta,s) for delta,s in candidates if delta <= duration]
        if not candidates:
            raise ValueError('完走時刻内の最終scoreが見つからない')
        selected = max(candidates,key=lambda item:item[0])[1]
    elif not load_end < selected.start() < finish:
        raise ValueError('LOAD END後の最終scoreが見つからない')
    score, gross, penalty, state = selected.groups()
    if int(score) != int(gross) - int(penalty):
        raise ValueError('最終score内訳が不一致')
    return int(score), state


def summarize(path):
    started = time.monotonic()
    timing = json.loads((path/'timing.json').read_text())
    start = int(timing['load']) + 1
    end = min(start + 59, int(timing['end']))
    if end <= start:
        raise ValueError('LOAD窓が空')
    score, state = final_score((path/'bench.log').read_text())
    if timing.get('returncode', 0) != 0:
        raise ValueError('ベンチ終了コードが非0')
    def status(name):
        return {k: int(v) for k, v in (line.split() for line in (path/name).read_text().splitlines())}
    before, after = status('db-before.txt'), status('db-after.txt')
    with concurrent.futures.ThreadPoolExecutor(4) as pool:
        sql = pool.submit(analyze, path/'slow.log.gz', start, end)
        flat = pool.submit(profile, path/'cpu.pprof', False)
        cum = pool.submit(profile, path/'cpu.pprof', True)
        metrics = pool.submit(cpu, path/'prometheus-cpu.json')
        valkey = None
        valkey_after = path/'valkey-after.txt'
        if valkey_after.is_file():
            valkey_before = path/'valkey-before.txt'
            valkey = analyze_valkey(valkey_before.read_text() if valkey_before.is_file() else '', valkey_after.read_text())
        result = dict(commit=timing['commit'], load=timing['load'], end=timing['end'], sql_window=[start,end], score=score, status=state, sql=sql.result(), flat=flat.result(), cumulative=cum.result(), cpu=metrics.result(), db_delta={k:after[k]-v for k,v in before.items() if not k.startswith('Threads_')}, valkey=valkey)
        for key, values in result['cpu'].items():
            if values['start'] is None or values['start'] < timing['load'] - 1 or values['end'] > timing['end'] + 1:
                raise ValueError('CPU時系列がLOAD窓の外: ' + key)
        result['analysis_seconds'] = time.monotonic() - started
        return result


def table(headers, rows):
    escape = lambda value: str(value).replace('|', '\\|').replace('\n', ' ')
    return '\n'.join(['|'+'|'.join(headers)+'|', '|'+'|'.join(['---']*len(headers))+'|'] + ['|'+'|'.join(map(escape,row))+'|' for row in rows])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--baseline', type=pathlib.Path, required=True)
    parser.add_argument('--candidate', type=pathlib.Path, required=True)
    parser.add_argument('--output', type=pathlib.Path, default=pathlib.Path('/tmp/isucon-work/report'))
    parser.add_argument('--pr', type=int, required=True)
    parser.add_argument('--issue', type=int, required=True)
    parser.add_argument('--repo', default='bgpat/isucon11q-20260905')
    parser.add_argument('--tag', required=True)
    parser.add_argument('--hypothesis', default='未評価。領域別実測と変更内容を照合する。')
    parser.add_argument('--decision', default='保留。自動集計のみでは採否・closeを決めない。')
    parser.add_argument('--publish', action='store_true')
    args = parser.parse_args()
    started = time.monotonic()
    args.output.mkdir(parents=True, exist_ok=True)
    if any(args.output.resolve().is_relative_to(p.resolve()) for p in (args.baseline,args.candidate)):
        raise ValueError('outputは入力ディレクトリの外を指定する')
    with concurrent.futures.ProcessPoolExecutor(2) as pool:
        pending = [pool.submit(summarize,p) for p in (args.baseline,args.candidate)]
        baseline,candidate = [f.result() for f in pending]
    analyzed = time.monotonic()
    if candidate['sql_window'][1]-candidate['sql_window'][0] != baseline['sql_window'][1]-baseline['sql_window'][0]:
        raise ValueError('SQL比較窓の長さが不一致')
    parts = [f'PR #{args.pr} 比較計測\n\n関連 Issue: https://github.com/{args.repo}/issues/{args.issue}\n\nRelease: https://github.com/{args.repo}/releases/tag/{args.tag}',
             '### 事実（実測）', table(['指標','baseline','candidate'],[['score',baseline['score'],candidate['score']],['結果',baseline['status'],candidate['status']],['exact commit',baseline['commit'],candidate['commit']],['SELECT Query_time合計 s',*[f"{sum(r['seconds'] for r in v['sql'] if r['sql'].startswith('SELECT ')):.6f}" for v in (baseline,candidate)]]]),
             'Query_timeは待機を含む経過時間でCPU時間ではない。pprofの比率は総CPU sampleが分母。flatは関数自身、cumulativeは子孫を含み重複するため合計不可。CPUは保存済みPrometheus rate系列の台形積分による推定値。DB差分には初期化が含まれる。Valkey の commandstats はベンチ前後スナップショットの差分（RESETSTAT した場合は after を区間値）。instantaneous_* は取得時点の値であり LOAD 平均ではない。']
    for label,data in [('baseline',baseline),('candidate',candidate)]:
        parts += ['### '+label+' 計測条件',json.dumps({k:data[k] for k in ('load','end','sql_window','commit')},ensure_ascii=False),
                  table(['プロセス(instance/group)','推定CPU秒','平均core','開始epoch','終了epoch'],[[k,f"{v['seconds']:.6f}",v['mean_cores'],v['start'],v['end']] for k,v in sorted(data['cpu'].items(),key=lambda item:-item[1]['seconds'])[:20]]),
                  table(['DB指標','差分'],data['db_delta'].items())]
        if data.get('valkey'):
            valkey = data['valkey']
            zset = valkey['zset']
            parts += ['### '+label+' Valkey commandstats top20（usec合計）',
                      'RESETSTAT 検知: '+('あり。after を区間値として使う。' if valkey['reset'] else 'なし。after-before 差分。')+' slowlog_len='+str(valkey['slowlog_len']),
                      table(['順位','コマンド','回数','usec合計','1回usec','rejected','failed'],
                            [[i,r['command'],r['calls'],r['usec'],(f"{r['usec_per_call']:.3f}" if r['usec_per_call'] is not None else ''),r['rejected'],r['failed']] for i,r in enumerate(valkey['commands'][:20],1)]),
                      '### '+label+' Valkey 統計',
                      table(['指標','before','after','差分','種別'],
                            [[r['key'],r['before'],r['after'],r['delta'],r['kind']] for r in valkey['stats']]),
                      '### '+label+' Valkey ZSET',
                      table(['指標','値'],[['zset_keys',zset['keys']],['members',zset['members']],['max',zset['max']],['p50',zset['p50']],['p95',zset['p95']],['latest_keys',zset['latest_keys']]])]
        parts += ['### '+label+' SQL top20',
                  table(['順位','SQL','回数','Query合計s','平均ms','最大ms','Lock合計s','Rows examined','Rows sent'],[[i,r['sql'],r['count'],f"{r['seconds']:.6f}",f"{r['seconds']/r['count']*1000:.6f}",f"{r['max_seconds']*1000:.3f}",f"{r['lock']:.6f}",r['examined'],r['sent']] for i,r in enumerate(data['sql'][:20],1)])]
        for mode in ('flat','cumulative'):
            parts += ['### '+label+' pprof '+mode+' top20',' / '.join(data[mode]['metadata']),table(['順位','関数','flat CPU','flat比','cumulative CPU','cumulative比'],[[i,*row] for i,row in enumerate(data[mode]['rows'],1)])]
        regions = [row for row in data['cumulative']['rows'] if row[0].startswith('main.') or 'Syscall6' in row[0]]
        parts += ['### '+label+' 領域別候補（cumulative上位20から抽出）',table(['優先','代表経路','flat CPU','flat比','cumulative CPU','cumulative比','扱い'],[[i,*row,'実測。採否は下記判断を参照'] for i,row in enumerate(regions,1)])]
    parts += ['### 推測・仮説',args.hypothesis,'### 判断',args.decision]
    (args.output/'metrics.json').write_text(json.dumps(dict(baseline=baseline,candidate=candidate),ensure_ascii=False,indent=2))
    (args.output/'report.md').write_text('\n\n'.join(parts)+'\n')
    composed = time.monotonic()
    with tarfile.open(args.output/'assets.tar.gz','w:gz',compresslevel=1) as archive:
        for label,path in [('baseline',args.baseline),('candidate',args.candidate)]:
            for name in ('bench.log','timing.json','cpu.pprof','slow.log.gz','access.log.gz','error.log','journal.log','db-before.txt','db-after.txt','prometheus-cpu.json','prometheus-db.json','state-before.txt','valkey-before.txt','valkey-after.txt'):
                if (path/name).is_file():
                    archive.add(path/name,arcname=label+'/'+name)
        for name in ('report.md','metrics.json'):
            archive.add(args.output/name,arcname=name)
    processing = dict(analysis_seconds=analyzed-started,compose_seconds=composed-analyzed,archive_seconds=time.monotonic()-composed,total_seconds=time.monotonic()-started,status='prepared')
    (args.output/'processing.json').write_text(json.dumps(processing))
    if args.publish:
        command('release-create',['gh','release','create',args.tag,'--repo',args.repo,'--target',candidate['commit'],'--title',f"#{args.pr}: {candidate['score']}点",'--notes-file',str(args.output/'report.md'),str(args.output/'assets.tar.gz'),str(args.output/'processing.json')])
        for kind,number in [('pr',args.pr),('issue',args.issue)]:
            command(kind+'-comment',['gh',kind,'comment',str(number),'--repo',args.repo,'--body-file',str(args.output/'report.md')])
        processing.update(status='complete',total_seconds=time.monotonic()-started)
        (args.output/'processing.json').write_text(json.dumps(processing))
        command('release-timing-update',['gh','release','upload',args.tag,'--repo',args.repo,'--clobber',str(args.output/'processing.json')])
    print(json.dumps(dict(report=str(args.output/'report.md'),seconds=time.monotonic()-started,published=args.publish)))


if __name__ == '__main__':
    main()
