-- Copyright The Linux Foundation and each contributor to LFX.
-- SPDX-License-Identifier: MIT

-- One-time cleanup of prod programs the legacy import left in review, agreed
-- with the Mentorship PM on 2026-11-10. Run once against the prod database:
--
--   psql "$PROD_DSN" -v ON_ERROR_STOP=1 -f backend/db/scripts/close_stale_pending_programs.sql
--
-- Plain SQL on purpose: the API decision path emails every program admin, and
-- these creators should not get a rejection years after they submitted. FGA
-- public is false for pending, hidden and rejected alike, so access is
-- unchanged; the OpenSearch status tag refreshes on each program's next write.
-- The set_updated_on trigger stamps updated_on.
--
-- The IDs were taken from prod on 2026-11-10. Each UPDATE re-checks the status,
-- and the transaction rolls back unless exactly 5 and 74 rows change, so
-- nothing is written if a listed program was decided or edited since.

BEGIN;
SET LOCAL search_path TO mentorship;

DO $$
DECLARE hidden_n int; rejected_n int;
BEGIN
  -- The legacy Hide button stored "hide", which the importer did not map, so
  -- these programs that admins had hidden arrived as pending.
  UPDATE programs SET status = 'hidden'
  WHERE status = 'pending' AND id IN (
    '8a8708df-72ee-4799-932e-9e6fe606bfc9',
    'a3b5fea2-4a2c-4a7b-97a0-656c978a01d2',
    'ba333574-1ce7-4fc7-9c56-901337672273',
    'c26f9189-4afa-414e-af39-b3dc73ebfef9',
    'c567fa35-dbe8-416f-9996-80cdeb9c1992'
  );
  GET DIAGNOSTICS hidden_n = ROW_COUNT;

  -- Legacy-pending programs last updated before 2026, the oldest from 2019. Old
  -- programs with a 2026 updated_on (the legacy term-status cron) stay pending.
  UPDATE programs SET status = 'rejected'
  WHERE status = 'pending' AND updated_on < '2026-01-01' AND id IN (
    '00465abe-5c68-4990-8b02-d261b3970949',
    '0161ed06-7d46-415f-9874-491c7fcef69b',
    '05497ec6-0542-4ff7-ad3f-6c6d292cb7fd',
    '05facefc-4f3b-4075-88b2-22599ce0ca3e',
    '09df1d30-e2a1-4cdf-b8ad-4650b686f4ec',
    '0c9bbe5b-7310-4895-a77b-a145b7682e30',
    '0e74bcb7-2415-48ea-ac8b-59af7cddb113',
    '18eb0ce7-20ee-46b5-b86e-ac361b16d06f',
    '21060041-3d58-43ef-99b5-04c3755ea4ee',
    '2199847c-f5e6-4287-850a-64b25f2cc246',
    '220b63a7-4ee4-4290-995c-c6441e5c44bb',
    '25a7d61a-b022-4936-a0d9-eddd3af3aeab',
    '2ee34e11-1245-40a9-9ea9-8a130fe9e680',
    '30f570f0-c3c7-424f-a1a5-2ef0f5f8188c',
    '41e7d7b7-528f-40f9-b2fb-e68e5f0dda86',
    '4366cfde-521c-4ba8-b86f-a052e9bddc9e',
    '4eb4ea89-beae-41f1-85c3-149ecadd1443',
    '51591b08-d3c9-4f98-9391-58fb8d596bf3',
    '591b1078-7ccf-4fb8-8b2a-00cc25099ca2',
    '61fe56b7-f069-43c1-a570-9214f7d007bc',
    '63ae2083-f0a7-4458-a279-e10b9662ea16',
    '6762e19b-0505-4822-9590-208c8c375007',
    '6b6b87e1-0bbb-4121-b276-fb8ea7d7df9b',
    '6e013091-5a31-46d1-a705-c0a06f6c6b68',
    '6e983ee6-509c-467d-b920-9aba6b5031dc',
    '70fe2a9c-b99d-4780-a7d4-b645a9c811a9',
    '71777df4-4bad-46e9-8813-693043a57ae4',
    '74f56e49-952a-435f-b993-7f67c8b21a0a',
    '76eba017-b111-445f-8cd2-a19342881834',
    '774e6528-b3da-4bb4-a5fe-568fcac0e311',
    '77f75cf6-343f-4af0-bf9b-d5c2e8000755',
    '798b2d79-e877-44cb-87a5-e0698f69241e',
    '80b71d76-8993-4706-a5a1-b332f94cf4c8',
    '816cb5a3-c267-4928-9145-7a79fbbae93c',
    '854f31cb-7300-4a6c-b7b2-8abf8ae64069',
    '864e5467-3c01-4477-b817-e52c01fa8883',
    '8658d763-9822-4621-ad7d-411100e5f486',
    '88e6bd6c-abb1-4910-823a-8f0420fbca90',
    '8b1a01c9-0962-491a-989b-8dd39d59b2a3',
    '91a94c32-a84b-4d62-a21c-6b32218623a0',
    '934ab384-9cf5-4e28-8a11-49557d182ed4',
    'a7112f5d-83bf-446e-bf49-5cb279cbd962',
    'ac771b5d-1ddd-4014-adb4-24da8ad67438',
    'acf1a56c-8617-4b6a-920c-86ce8cb04a3f',
    'ad9adcec-a739-4d7b-a851-2fba2e89d8f6',
    'ade746e3-607f-4d55-acb5-56e3f63f9be9',
    'ae3282c8-d51d-4273-9e3b-3fb1d2798ac9',
    'aeffdbf1-bda8-4c45-9de3-66e17974de20',
    'b69cfd31-e9c7-4cf6-927e-48b3e97156c6',
    'b7b26d5b-e3b5-4367-8d7d-bbe46d510145',
    'b8457705-15ea-4527-b032-f272bfee85ba',
    'bd8c2f8b-803c-49d8-9dba-f70103b7c1db',
    'be2f967c-227f-4932-97cf-14a24c108291',
    'bfae50aa-299c-438e-a43f-5e2e7239242a',
    'ce86aa5b-8948-4347-813b-04dea4c3040d',
    'd00f6282-7195-4c0e-b84f-f5e045c91c4a',
    'd6e57229-4ba4-49ab-8c7c-8e77437a39e9',
    'd9ee59f3-0014-47a4-b9b9-99bf12346038',
    'db3bc27b-d66c-4350-9480-61158d6e2134',
    'db98e5e5-c98c-42c9-b2dc-dcd64cda9bc8',
    'dc0e1e60-c73e-4f3a-a4e1-baacddfc764f',
    'dfe43031-c03a-43b5-bc5c-29dfa814fe03',
    'e2698a5c-bc7d-4985-b534-f6cc78e5b1e9',
    'e32f3eed-a8c6-4049-8d2f-91a5b46afc83',
    'e359f653-d525-4635-b43f-810fdd2bd895',
    'e6cf36e9-82a5-4fb0-ae75-39b0ef887c20',
    'e87030a6-f7f3-4667-8818-4036249a670e',
    'eca0c2a7-3166-4c3d-a04d-f5d12db6b06c',
    'ecdb0978-5ea2-456e-b296-9f371f1db399',
    'ef95573f-ff62-4d60-9249-fde87a6145f6',
    'f3c7b9ec-ecdc-4b00-9eed-bb64c4ecdd22',
    'f5387733-28d0-4ef1-b555-cb2b728dc44b',
    'fa6f1f15-8fb7-49b2-a5f4-59b591bc0518',
    'fc326644-e813-4c96-9eec-5ede10ceba41'
  );
  GET DIAGNOSTICS rejected_n = ROW_COUNT;

  IF hidden_n <> 5 OR rejected_n <> 74 THEN
    RAISE EXCEPTION 'expected 5 hidden and 74 rejected, got % and %; nothing was changed', hidden_n, rejected_n;
  END IF;
  RAISE NOTICE 'hidden: %, rejected: %', hidden_n, rejected_n;
END $$;

COMMIT;
