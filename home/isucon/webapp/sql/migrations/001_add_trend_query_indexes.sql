-- getTrend が ISU ごとの最新 condition を取得するための複合インデックス。
-- timestamp が同一の行は id の降順で決定する。
ALTER TABLE `isu_condition`
  ADD INDEX `idx_isu_condition_jia_isu_uuid_timestamp_id` (`jia_isu_uuid`, `timestamp` DESC, `id` DESC);
