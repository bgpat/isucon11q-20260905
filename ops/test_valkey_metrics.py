import unittest
from valkey_metrics import analyze, parse_snapshot

BEFORE = """# valkey-snapshot
# collected_at=2026-09-07T12:00:00Z
# slowlog_len=1
=== INFO all ===
total_commands_processed:1000
total_net_output_bytes:5000
aof_delayed_fsync:2
aof_current_size:100
used_memory:200
cmdstat_zadd:calls=10,usec=100,usec_per_call=10.00,rejected_calls=0,failed_calls=0
=== ZCARD cond:z:* ===
cond:z:a 5
# latest_keys=1
"""

AFTER_RESET = """# valkey-snapshot
# collected_at=2026-09-07T12:02:00Z
# slowlog_len=0
=== INFO all ===
total_commands_processed:40
total_net_output_bytes:80000
aof_delayed_fsync:3
aof_current_size:180
used_memory:400
instantaneous_output_kbps:12.5
cmdstat_zadd:calls=30,usec=300,usec_per_call=10.00,rejected_calls=0,failed_calls=0
cmdstat_zrevrangebyscore:calls=10,usec=5000,usec_per_call=500.00,rejected_calls=0,failed_calls=0
=== ZCARD cond:z:* ===
cond:z:a 10
cond:z:b 90
# latest_keys=2
"""

AFTER_NO_RESET = """# valkey-snapshot
# slowlog_len=2
=== INFO all ===
total_commands_processed:1040
total_net_output_bytes:85000
aof_delayed_fsync:3
aof_current_size:180
used_memory:400
cmdstat_zadd:calls=40,usec=400,usec_per_call=10.00,rejected_calls=0,failed_calls=0
cmdstat_zrevrangebyscore:calls=10,usec=5000,usec_per_call=500.00,rejected_calls=0,failed_calls=0
=== ZCARD cond:z:* ===
cond:z:a 10
# latest_keys=1
"""


class ValkeyMetricsTest(unittest.TestCase):
    def test_parse_zcard_and_cmdstat(self):
        parsed = parse_snapshot(AFTER_RESET)
        self.assertEqual(parsed['commands']['zrevrangebyscore']['usec'], 5000)
        self.assertEqual(parsed['zcards'], [10, 90])
        self.assertEqual(parsed['latest_keys'], 2)
        self.assertEqual(parsed['slowlog_len'], 0)

    def test_reset_uses_after_as_interval(self):
        result = analyze(BEFORE, AFTER_RESET)
        self.assertTrue(result['reset'])
        by_name = {row['command']: row for row in result['commands']}
        self.assertEqual(by_name['zadd']['calls'], 30)
        self.assertEqual(by_name['zrevrangebyscore']['usec'], 5000)
        stats = {row['key']: row for row in result['stats']}
        self.assertEqual(stats['total_net_output_bytes']['delta'], 80000)
        self.assertEqual(stats['aof_delayed_fsync']['delta'], 1)
        self.assertEqual(stats['used_memory']['before'], 200)
        self.assertEqual(stats['used_memory']['after'], 400)
        self.assertEqual(result['zset']['members'], 100)
        self.assertEqual(result['zset']['max'], 90)
        self.assertEqual(result['zset']['p50'], 50)

    def test_no_reset_uses_delta(self):
        result = analyze(BEFORE, AFTER_NO_RESET)
        self.assertFalse(result['reset'])
        by_name = {row['command']: row for row in result['commands']}
        self.assertEqual(by_name['zadd']['calls'], 30)
        stats = {row['key']: row for row in result['stats']}
        self.assertEqual(stats['total_net_output_bytes']['delta'], 80000)

    def test_after_only_treats_zero_baseline(self):
        result = analyze('', AFTER_RESET)
        self.assertFalse(result['reset'])
        by_name = {row['command']: row for row in result['commands']}
        self.assertEqual(by_name['zadd']['calls'], 30)

    def test_empty_after_is_error(self):
        with self.assertRaises(ValueError):
            analyze(BEFORE, '')


if __name__ == '__main__':
    unittest.main()
