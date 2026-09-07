import gzip
import tempfile
import pathlib
import unittest
import importlib.util
from benchmark_sql import analyze

spec = importlib.util.spec_from_file_location('report', pathlib.Path(__file__).with_name('benchmark-report.py'))
report = importlib.util.module_from_spec(spec)
spec.loader.exec_module(report)


class SQLTest(unittest.TestCase):
    def test_window_normalization_and_last_record(self):
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory)/'slow.log.gz'
            with gzip.open(path,'wt') as output:
                for stamp in (99,100,101,102):
                    output.write(f"# Query_time: 0.200000 Lock_time: 0.010000 Rows_sent: 2 Rows_examined: 3\nSET timestamp={stamp};\nSELECT * FROM t WHERE uuid='u{stamp}';\n")
            rows = analyze(path,100,102)
            self.assertEqual(len(rows),1)
            self.assertEqual(rows[0]['count'],2)
            self.assertAlmostEqual(rows[0]['seconds'],0.4)
            self.assertAlmostEqual(rows[0]['max_seconds'],0.2)
            self.assertEqual(rows[0]['examined'],6)
            self.assertEqual(analyze(path,102,103)[0]['count'],1)

    def test_completed_score(self):
        log = '<=== LOAD END\nscore: 99(100 - 1) : pass\n<=== sendResult finish'
        self.assertEqual(report.final_score(log),(99,'pass'))
        for invalid in (log.replace('<=== LOAD END',''), log.replace('<=== sendResult finish',''),
                        log.replace('99(100','98(100'), 'score: 99(100 - 1) : pass\n<=== LOAD END\n<=== sendResult finish'):
            with self.assertRaises(ValueError):
                report.final_score(invalid)

    def test_interleaved_streams(self):
        log = ('22:26:06.811001 score: 201602(201654 - 52) : pass\n'
               '22:26:06.810346 <=== LOAD END\n'
               '22:26:06.811007 <=== sendResult finish\n')
        self.assertEqual(report.final_score(log),(201602,'pass'))
        with self.assertRaises(ValueError):
            report.final_score(log.replace('22:26:06.811001','22:26:03.811001'))

    def test_midnight_and_stale_score(self):
        log = ('00:00:00.001 score: 99(100 - 1) : pass\n'
               '23:59:59.990 <=== LOAD END\n'
               '23:59:56.000 score: 89(90 - 1) : pass\n'
               '00:00:00.002 <=== sendResult finish\n')
        self.assertEqual(report.final_score(log),(99,'pass'))


if __name__ == '__main__':
    unittest.main()
