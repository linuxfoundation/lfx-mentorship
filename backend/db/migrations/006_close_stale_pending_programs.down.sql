-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

-- 006 is a data fix with no inverse: putting the programs back in review would
-- reopen the backlog it closed. This file only lets the schema step back past
-- 006 (migration_005_integration_test.go migrates down to 4).
SELECT 1;
