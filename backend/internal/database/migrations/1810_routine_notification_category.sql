-- Exam routine notifications: the 'routine' category joins notifications,
-- preferences and the SMS log (the dispatcher sends it in-app only today,
-- but the preference row allows SMS like every category).
ALTER TABLE notifications DROP CONSTRAINT notifications_category_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_category_check CHECK (category IN ('notice', 'absence', 'result', 'fee', 'diary', 'routine'));
ALTER TABLE notification_preferences DROP CONSTRAINT notification_preferences_category_check;
ALTER TABLE notification_preferences ADD CONSTRAINT notification_preferences_category_check CHECK (category IN ('notice', 'absence', 'result', 'fee', 'diary', 'routine'));
ALTER TABLE sms_messages DROP CONSTRAINT sms_messages_category_check;
ALTER TABLE sms_messages ADD CONSTRAINT sms_messages_category_check CHECK (category IN ('notice', 'absence', 'result', 'fee', 'diary', 'routine'));
