CREATE TABLE reaction_instances (
    id   BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    message_id BIGINT,
    reaction VARCHAR(16)
);
