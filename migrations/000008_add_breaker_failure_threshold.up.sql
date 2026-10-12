ALTER TABLE providers
    ADD COLUMN breaker_failure_threshold INT NOT NULL DEFAULT 5
        CHECK (breaker_failure_threshold >= 1);