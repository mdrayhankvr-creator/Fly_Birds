-- Promotion carries the group and the fourth subject into the next year.
-- Each item records the student's next-year group and fourth subject,
-- prefilled from the closing year when the batch is built and editable while
-- the batch is a draft; applying the batch writes them as the new session's
-- choices (only where the target class uses them).
ALTER TABLE promotion_items
    ADD COLUMN target_group_id uuid REFERENCES academic_groups(id) ON DELETE RESTRICT,
    ADD COLUMN target_fourth_subject_id uuid REFERENCES academic_subjects(id) ON DELETE RESTRICT;
