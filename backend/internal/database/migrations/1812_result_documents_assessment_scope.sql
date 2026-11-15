-- Fix: a marksheet of an exam's own published result could not be made.
--
-- result_documents (1800) was written when documents covered term and annual
-- results only, so its scope check allows 'term' and 'annual'. Transcript v3
-- added marksheets for an exam's own published result ('assessment'), which
-- result_versions and result_publications (1503) have always allowed, but the
-- check on result_documents was never widened. Every request for such a
-- marksheet — a student downloading their class-test result, or staff
-- printing it — failed on INSERT with a check violation and the server
-- answered 500; nothing was rendered.
--
-- Widening the check is enough: the row's other constraints already fit an
-- exam-scope marksheet (student_id is set for kind = 'marksheet', and
-- session_id is the exam's session). Class documents stay term- and
-- annual-only, which the service enforces. No existing row changes: no
-- 'assessment' document could ever have been stored.

ALTER TABLE result_documents DROP CONSTRAINT IF EXISTS result_documents_scope_check;
ALTER TABLE result_documents DROP CONSTRAINT IF EXISTS result_documents_scope_ck;
ALTER TABLE result_documents
    ADD CONSTRAINT result_documents_scope_ck CHECK (scope IN ('assessment', 'term', 'annual'));
