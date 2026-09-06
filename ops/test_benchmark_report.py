import gzip
import tempfile
import pathlib
import unittest
from benchmark_sql import analyze


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
            self.assertEqual(rows[0]['examined'],6)
            self.assertEqual(analyze(path,102,103)[0]['count'],1)


if __name__ == '__main__':
    unittest.main()
