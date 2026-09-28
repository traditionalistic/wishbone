-- A screen taken off the air: GET /v1/screens answers SCREEN_NOT_FOUND until the next publish/rollback.
ALTER TABLE screens ADD COLUMN unpublished_at timestamptz;
