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
    return dict(raw=raw, rows=rows)


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
        result[key] = dict(seconds=seconds, mean_cores=seconds/(items[-1][0]-items[0][0]) if len(items)>1 else None)
    return result


def summarize(path):
    started = time.monotonic()
    timing = json.loads((path/'timing.json').read_text())
    start = int(timing['load']) + 1
    end = min(start + 59, int(timing['end']))
    if end <= start:
        raise ValueError('LOAD窓が空')
    scores = re.findall(r'score:\s*(\d+)\([^\n]*?\)\s*:\s*(\w+)', (path/'bench.log').read_text())
    if not scores:
        raise ValueError('最終scoreが見つからない: ' + str(path))
    def status(name):
        return {k: int(v) for k, v in (line.split() for line in (path/name).read_text().splitlines())}
    before, after = status('db-before.txt'), status('db-after.txt')
    with concurrent.futures.ThreadPoolExecutor(4) as pool:
        sql = pool.submit(analyze, path/'slow.log.gz', start, end)
        flat = pool.submit(profile, path/'cpu.pprof', False)
        cum = pool.submit(profile, path/'cpu.pprof', True)
        metrics = pool.submit(cpu, path/'prometheus-cpu.json')
        result = dict(commit=timing['commit'], load=timing['load'], end=timing['end'], sql_window=[start,end], score=int(scores[-1][0]), status=scores[-1][1], sql=sql.result(), flat=flat.result(), cumulative=cum.result(), cpu=metrics.result(), db_delta={k:after[k]-v for k,v in before.items() if not k.startswith('Threads_')})
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
             'Query_timeは待機を含む経過時間でCPU時間ではない。pprofの比率は総CPU sampleが分母。flatは関数自身、cumulativeは子孫を含み重複するため合計不可。CPUは保存済みPrometheus rate系列の台形積分による推定値。DB差分には初期化が含まれる。']
    for label,data in [('baseline',baseline),('candidate',candidate)]:
        parts += ['### '+label+' 計測条件',json.dumps({k:data[k] for k in ('load','end','sql_window','commit')},ensure_ascii=False),
                  table(['プロセス(instance/group)','推定CPU秒','平均core'],[[k,f"{v['seconds']:.6f}",v['mean_cores']] for k,v in sorted(data['cpu'].items(),key=lambda item:-item[1]['seconds'])[:20]]),
                  table(['DB指標','差分'],data['db_delta'].items()),'### '+label+' SQL top20',
                  table(['順位','SQL','回数','Query合計s','平均ms','Lock合計s','Rows examined','Rows sent'],[[i,r['sql'],r['count'],f"{r['seconds']:.6f}",f"{r['seconds']/r['count']*1000:.6f}",f"{r['lock']:.6f}",r['examined'],r['sent']] for i,r in enumerate(data['sql'][:20],1)])]
        for mode in ('flat','cumulative'):
            parts += ['### '+label+' pprof '+mode+' top20',table(['順位','関数','flat CPU','flat比','cumulative CPU','cumulative比'],[[i,*row] for i,row in enumerate(data[mode]['rows'],1)])]
        regions = [row for row in data['cumulative']['rows'] if row[0].startswith('main.') or 'Syscall6' in row[0]]
        parts += ['### '+label+' 領域別候補（cumulative上位20から抽出）',table(['優先','代表経路','flat CPU','flat比','cumulative CPU','cumulative比','扱い'],[[i,*row,'実測。採否は下記判断を参照'] for i,row in enumerate(regions,1)])]
    parts += ['### 推測・仮説',args.hypothesis,'### 判断',args.decision]
    (args.output/'metrics.json').write_text(json.dumps(dict(baseline=baseline,candidate=candidate),ensure_ascii=False,indent=2))
    (args.output/'report.md').write_text('\n\n'.join(parts)+'\n')
    composed = time.monotonic()
    with tarfile.open(args.output/'assets.tar.gz','w:gz',compresslevel=1) as archive:
        for label,path in [('baseline',args.baseline),('candidate',args.candidate)]:
            for name in ('bench.log','timing.json','cpu.pprof','slow.log.gz','access.log.gz','error.log','journal.log','db-before.txt','db-after.txt','prometheus-cpu.json','prometheus-db.json','state-before.txt'):
                if (path/name).is_file():
                    archive.add(path/name,arcname=label+'/'+name)
        for name in ('report.md','metrics.json'):
            archive.add(args.output/name,arcname=name)
    (args.output/'processing.json').write_text(json.dumps(dict(analysis_seconds=analyzed-started,compose_seconds=composed-analyzed,archive_seconds=time.monotonic()-composed,total_seconds=time.monotonic()-started,published=args.publish)))
    if args.publish:
        command('release-create',['gh','release','create',args.tag,'--repo',args.repo,'--target',candidate['commit'],'--title',f"#{args.pr}: {candidate['score']}点",'--notes-file',str(args.output/'report.md'),str(args.output/'assets.tar.gz'),str(args.output/'processing.json')])
        for kind,number in [('pr',args.pr),('issue',args.issue)]:
            command(kind+'-comment',['gh',kind,'comment',str(number),'--repo',args.repo,'--body-file',str(args.output/'report.md')])
    print(json.dumps(dict(report=str(args.output/'report.md'),seconds=time.monotonic()-started,published=args.publish)))


if __name__ == '__main__':
    main()
