-- グラフ取得が参照する列をインデックスだけで取得できるようにする。
-- jia_isu_uuid と timestamp は検索・並び順、残り2列はcovering用。
ALTER TABLE `isu_condition`
  ADD INDEX `idx_isu_condition_graph_cover` (`jia_isu_uuid`, `timestamp`, `is_sitting`, `condition`);
