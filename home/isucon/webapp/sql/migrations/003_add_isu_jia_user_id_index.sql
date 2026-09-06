-- getIsuList が jia_user_id で isu を絞ってから最新 condition を取るための索引。
-- isu_condition の (jia_isu_uuid, timestamp DESC, id DESC) は変更しない。
ALTER TABLE `isu`
  ADD INDEX `idx_isu_jia_user_id` (`jia_user_id`);
