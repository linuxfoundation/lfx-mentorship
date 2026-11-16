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
-- The IDs were taken from prod on 2026-11-11; none was updated after the
-- 2026-10-27 import. Each UPDATE re-checks the status and that cutoff, and the
-- transaction rolls back unless exactly 5 and 122 rows change, so nothing is
-- written if a listed program was decided or edited since.

BEGIN;
SET LOCAL search_path TO mentorship;

DO $$
DECLARE hidden_n int; rejected_n int;
BEGIN
  -- The legacy Hide button stored "hide", which the importer did not map, so
  -- these programs that admins had hidden arrived as pending.
  UPDATE programs SET status = 'hidden'
  WHERE status = 'pending' AND updated_on < '2026-10-27' AND id IN (
    '8a8708df-72ee-4799-932e-9e6fe606bfc9',
    'a3b5fea2-4a2c-4a7b-97a0-656c978a01d2',
    'ba333574-1ce7-4fc7-9c56-901337672273',
    'c26f9189-4afa-414e-af39-b3dc73ebfef9',
    'c567fa35-dbe8-416f-9996-80cdeb9c1992'
  );
  GET DIAGNOSTICS hidden_n = ROW_COUNT;

  -- Legacy-pending programs submitted before 2026, the oldest from 2019. 48 of
  -- them show a 2026 updated_on only because the legacy term-status cron
  -- rewrote the project record.
  UPDATE programs SET status = 'rejected'
  WHERE status = 'pending' AND updated_on < '2026-10-27' AND id IN (
    '00465abe-5c68-4990-8b02-d261b3970949',
    '0161ed06-7d46-415f-9874-491c7fcef69b',
    '05497ec6-0542-4ff7-ad3f-6c6d292cb7fd',
    '05facefc-4f3b-4075-88b2-22599ce0ca3e',
    '09df1d30-e2a1-4cdf-b8ad-4650b686f4ec',
    '0b5b33ad-a9b9-4c1d-988d-3c1c92462b5f',
    '0c9bbe5b-7310-4895-a77b-a145b7682e30',
    '0e74bcb7-2415-48ea-ac8b-59af7cddb113',
    '0e9d10d3-9288-426d-bc21-023cfc49453f',
    '12bcae4e-a81e-40b1-9183-2f0887e8f2e7',
    '16899dbe-468e-47db-9230-5d7c4fca4533',
    '16e04472-1cf4-4da3-b26c-e7f67cc82e94',
    '18eb0ce7-20ee-46b5-b86e-ac361b16d06f',
    '1a2501b0-95b3-4982-8e77-8181c68ada9f',
    '1aea5390-591a-4075-bfb5-53641ae4110e',
    '1d6b29e1-93f8-4e37-8197-0264e732b1aa',
    '20bbc218-bc41-4aa9-aaee-eea490bbda41',
    '21060041-3d58-43ef-99b5-04c3755ea4ee',
    '2199847c-f5e6-4287-850a-64b25f2cc246',
    '220b63a7-4ee4-4290-995c-c6441e5c44bb',
    '22862b7f-f8a6-4401-9907-f6026afad145',
    '25499b77-38e5-4bb6-8a6b-6b038ef1df85',
    '25a7d61a-b022-4936-a0d9-eddd3af3aeab',
    '2ee34e11-1245-40a9-9ea9-8a130fe9e680',
    '30f570f0-c3c7-424f-a1a5-2ef0f5f8188c',
    '36444a2e-211a-465b-9e0e-c29b920b1876',
    '3b918021-8a0e-4843-83f7-c575949d1d5c',
    '41e7d7b7-528f-40f9-b2fb-e68e5f0dda86',
    '4366cfde-521c-4ba8-b86f-a052e9bddc9e',
    '4eb4ea89-beae-41f1-85c3-149ecadd1443',
    '51591b08-d3c9-4f98-9391-58fb8d596bf3',
    '515e26a1-9e56-49e6-bb87-ce5544e8ee10',
    '51aa8d51-24e5-4bef-a599-ef550d0dd458',
    '530f4263-5858-4ad8-b1be-31d62cc18550',
    '591b1078-7ccf-4fb8-8b2a-00cc25099ca2',
    '5998326f-2bc4-437a-b9fc-70174a03225b',
    '61129c5c-bcaa-4bf0-823b-d4d4079b6060',
    '61fe56b7-f069-43c1-a570-9214f7d007bc',
    '63ae2083-f0a7-4458-a279-e10b9662ea16',
    '650ab5f1-1855-4234-8e4d-253f5b679b1e',
    '6762e19b-0505-4822-9590-208c8c375007',
    '6a4ce9f3-2052-4bc9-9e0e-ab48f1f9c390',
    '6b6b87e1-0bbb-4121-b276-fb8ea7d7df9b',
    '6e013091-5a31-46d1-a705-c0a06f6c6b68',
    '6e983ee6-509c-467d-b920-9aba6b5031dc',
    '6f083821-f90c-4ff8-940d-f246bc0ceefe',
    '70fe2a9c-b99d-4780-a7d4-b645a9c811a9',
    '71777df4-4bad-46e9-8813-693043a57ae4',
    '74f56e49-952a-435f-b993-7f67c8b21a0a',
    '754cd85c-b503-491c-840e-b8bbbd35fe5f',
    '76eba017-b111-445f-8cd2-a19342881834',
    '774e6528-b3da-4bb4-a5fe-568fcac0e311',
    '77f75cf6-343f-4af0-bf9b-d5c2e8000755',
    '798b2d79-e877-44cb-87a5-e0698f69241e',
    '7b6b9094-b3fe-4cba-beee-e6da4a148ac6',
    '7d928491-4f44-47c7-a560-1f7e759d3a7a',
    '80b71d76-8993-4706-a5a1-b332f94cf4c8',
    '816cb5a3-c267-4928-9145-7a79fbbae93c',
    '83e64e42-fd11-4882-9837-2d66e6e98be5',
    '854f31cb-7300-4a6c-b7b2-8abf8ae64069',
    '864e5467-3c01-4477-b817-e52c01fa8883',
    '8658d763-9822-4621-ad7d-411100e5f486',
    '88e6bd6c-abb1-4910-823a-8f0420fbca90',
    '8b1a01c9-0962-491a-989b-8dd39d59b2a3',
    '8b42bb9b-6ce1-4aea-988b-7fa1fe363b75',
    '8e2eabf5-d267-4ba0-b7f8-8738c93a8d6f',
    '8f3e44fb-1649-4578-a01d-4e6317e66bdd',
    '91a94c32-a84b-4d62-a21c-6b32218623a0',
    '934ab384-9cf5-4e28-8a11-49557d182ed4',
    '94394303-f6cd-4d5f-be17-4a0d48d623ea',
    '9a57dde6-dec9-41b8-8299-54429d079733',
    '9d321186-d195-42bd-b1d6-3b1df3245fea',
    'a1b38c00-10c4-4d10-8762-f711ede1dab3',
    'a363620e-1fb7-4f44-bbe9-b8de94feebae',
    'a7112f5d-83bf-446e-bf49-5cb279cbd962',
    'aa32cad5-7103-4a6a-95f3-9adb76664801',
    'aa933e63-ae3d-4cd0-b654-f84b39d74f86',
    'ac771b5d-1ddd-4014-adb4-24da8ad67438',
    'acf1a56c-8617-4b6a-920c-86ce8cb04a3f',
    'ad9adcec-a739-4d7b-a851-2fba2e89d8f6',
    'ade746e3-607f-4d55-acb5-56e3f63f9be9',
    'ae3282c8-d51d-4273-9e3b-3fb1d2798ac9',
    'aeffdbf1-bda8-4c45-9de3-66e17974de20',
    'b69cfd31-e9c7-4cf6-927e-48b3e97156c6',
    'b7b26d5b-e3b5-4367-8d7d-bbe46d510145',
    'b7f784cf-9b26-4bf9-9201-8c6e2584dbfb',
    'b8457705-15ea-4527-b032-f272bfee85ba',
    'b979e889-329f-49d5-aee1-42342b793de8',
    'bd8c2f8b-803c-49d8-9dba-f70103b7c1db',
    'be2f967c-227f-4932-97cf-14a24c108291',
    'bfae50aa-299c-438e-a43f-5e2e7239242a',
    'ca10a219-baf7-40b6-bf4a-37862ce904dd',
    'cc183437-7d11-41f3-ab6f-015c63629b44',
    'cc45aaf1-9b24-4fc3-a2eb-cda6f583369d',
    'ce86aa5b-8948-4347-813b-04dea4c3040d',
    'ce87db2e-5e66-47a1-9ea1-86f75f478dfb',
    'd00f6282-7195-4c0e-b84f-f5e045c91c4a',
    'd315e2ad-f5ea-446f-bd09-b2b28eeb9dfd',
    'd6e57229-4ba4-49ab-8c7c-8e77437a39e9',
    'd9ee59f3-0014-47a4-b9b9-99bf12346038',
    'db0ea9d9-6b6c-4985-bb07-65541bc1337b',
    'db199a8e-b0bc-47aa-a90f-85fdf8693d3d',
    'db3bc27b-d66c-4350-9480-61158d6e2134',
    'db98e5e5-c98c-42c9-b2dc-dcd64cda9bc8',
    'dc0e1e60-c73e-4f3a-a4e1-baacddfc764f',
    'dfe43031-c03a-43b5-bc5c-29dfa814fe03',
    'e0376696-8a0c-4df8-9ea2-87ee1517d698',
    'e2698a5c-bc7d-4985-b534-f6cc78e5b1e9',
    'e32f3eed-a8c6-4049-8d2f-91a5b46afc83',
    'e359f653-d525-4635-b43f-810fdd2bd895',
    'e6cf36e9-82a5-4fb0-ae75-39b0ef887c20',
    'e87030a6-f7f3-4667-8818-4036249a670e',
    'eca0c2a7-3166-4c3d-a04d-f5d12db6b06c',
    'ecdb0978-5ea2-456e-b296-9f371f1db399',
    'ef95573f-ff62-4d60-9249-fde87a6145f6',
    'f09c60ec-07cc-48e6-ac78-52a7c24f579b',
    'f2b149c9-49fe-4ba5-bf70-8da5c4d6c311',
    'f3c7b9ec-ecdc-4b00-9eed-bb64c4ecdd22',
    'f5387733-28d0-4ef1-b555-cb2b728dc44b',
    'f9e0b829-ebfd-418d-bb46-315bf1d740f9',
    'fa6f1f15-8fb7-49b2-a5f4-59b591bc0518',
    'fc326644-e813-4c96-9eec-5ede10ceba41'
  );
  GET DIAGNOSTICS rejected_n = ROW_COUNT;

  IF hidden_n <> 5 OR rejected_n <> 122 THEN
    RAISE EXCEPTION 'expected 5 hidden and 122 rejected, got % and %; nothing was changed', hidden_n, rejected_n;
  END IF;
  RAISE NOTICE 'hidden: %, rejected: %', hidden_n, rejected_n;
END $$;

COMMIT;
