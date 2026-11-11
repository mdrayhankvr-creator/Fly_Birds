// Marksheet v2 and admit cards: class teachers, comments, signatures and
// admit cards.
export const marksheetAdmitTranslations = {
  en: {
    msNavComments: 'Class teacher comments', msNavAdmit: 'Admit cards',
    msLoading: 'Loading…', msLoadError: 'Could not load this. Try again.', msErrGeneric: 'The change could not be saved.', msErrForbidden: 'You may not make this change.',
    msSave: 'Save', msSaved: 'Saved.', msChoose: 'Choose…', msEnglish: 'English', msBangla: 'বাংলা',

    msCommentsTitle: 'Class teacher comments',
    msCommentsIntro: 'Write a short comment for each student’s marksheet: choose a preset or write your own. The head teacher reviews them. A comment never changes a published result; the next marksheet shows it.',
    msCommentsIntroReviewer: 'Review what class teachers wrote on each student’s marksheet. You can correct a comment; your change counts as reviewed.',
    msSection: 'Section', msResult: 'Result', msAnnualOf: 'Annual result {{session}}', msClassTeacherLine: 'Class teacher: {{name}}', msNoClassTeacher: 'No class teacher assigned',
    msNoSections: 'You are not class teacher of a section yet.', msNoSectionsHint: 'The administrator assigns class teachers on the Sections page.',
    msStudent: 'Student', msRoll: 'Roll', msComment: 'Comment', msStatus: 'Status', msGpa: 'GPA',
    msNoComment: 'No comment', msOwnWords: 'Own words…', msOwnPlaceholder: 'A short comment, up to 400 characters', msCommentFor: 'Comment for {{name}}', msOwnFor: 'Own words for {{name}}',
    msCleared: 'Comment removed.',
    msStatus_written: 'Written', msStatus_reviewed: 'Reviewed', msStatus_none: 'Not written', msWrittenBy: 'By {{name}}',
    msReviewAll: 'Mark written comments reviewed', msReviewedCount: '{{n}} comments marked reviewed.', msNothingToReview: 'Nothing waits for review.',
    msPresetsTitle: 'Preset comments', msPresetsIntro: 'Class teachers choose from these. A changed preset prints its new words on every marksheet made from now on.',
    msPresetEN: 'English', msPresetBN: 'Bangla', msPresetOrder: 'Order', msPresetActive: 'In use', msPresetAdd: 'Add a preset', msPresetSave: 'Save preset', msPresetNew: 'New preset',

    msClassTeachersTitle: 'Class teachers', msClassTeachersIntro: 'One class teacher per section. They write the comments on their students’ marksheets and print their admit cards; their signature is printed on the marksheet.',
    msSession: 'Session', msTeacher: 'Class teacher', msNoTeacher: 'Not assigned', msSignatureMissing: 'No signature uploaded yet', msSignatureReady: 'Signature uploaded', msAssigned: 'Class teacher saved.',
    msTeacherFor: 'Class teacher of {{section}}',

    msSignatureTitle: 'My signature', msSignatureIntro: 'Printed on the marksheets of the sections you are class teacher of. Upload a PNG with a transparent background, or a clear photo or scan of your signature on white paper: the paper is removed, and the image is trimmed and resized.',
    msSignatureUpload: 'Upload signature', msSignatureReplace: 'Replace signature', msSignatureRemove: 'Remove', msSignatureNone: 'No signature yet.', msSignatureSaved: 'Signature saved.', msSignatureRemoved: 'Signature removed.',
    msSignatureInvalid: 'Use a PNG or JPEG image (at most 4 MB) with a visible signature.', msSignatureAlt: 'Your signature as it is printed',

    msAdmitTitle: 'Admit cards', msAdmitIntro: 'Admit cards are made from each term’s published exam routine: every student’s own papers, room and seat, with the rules and the head teacher’s signature. Two cards per A4 sheet.',
    msAdmitTerm: 'Exam term', msAdmitNoTerms: 'No published exam routine yet.', msAdmitNoTermsHint: 'Admit cards appear here once a term’s routine is published.', msAdmitVersion: 'Routine version {{n}}',
    msAdmitClass: 'Class', msAdmitStudents: 'Students', msAdmitHeldCol: 'Held', msAdmitPrintSection: 'Print {{class}} {{section}} in {{language}}', msAdmitWholeClass: 'Whole class {{class}}',
    msAdmitPreparing: 'Preparing the admit cards…', msAdmitHeldOut: 'Downloaded. {{n}} students held for dues were left out.', msAdmitDone: 'Downloaded.',
    msAdmitHeldTitle: 'Held for dues', msAdmitHeldIntro: 'The fee hold is on: a student with a due unpaid on or before {{date}} gets no admit card until the office releases them.',
    msAdmitHeldOff: 'The fee hold is off: every student gets their card.', msAdmitNoHeld: 'No student has a due by the cutoff.',
    msAdmitDue: 'Due', msAdmitOldest: 'Oldest due', msAdmitRelease: 'Release', msAdmitReleasedState: 'Released', msAdmitHeldState: 'Held', msAdmitHoldAgain: 'Hold again', msAdmitNote: 'Note (optional)', msAdmitNoteFor: 'Release note for {{name}}',
    msAdmitReleasedMsg: 'Released: {{name}} can have the admit card.', msAdmitHeldMsg: '{{name}} is held again.',
    msAdmitSettingsTitle: 'Rules and fee hold', msAdmitRulesEN: 'Rules in English, one per line', msAdmitRulesBN: 'Rules in Bangla, one per line', msAdmitRulesHint: 'Leave empty to print the built-in rules shown as the placeholder.',
    msAdmitHold: 'Hold admit cards when fees are due', msAdmitHoldHint: 'Off by default. The office can release individual students.', msAdmitCutoff: 'Dues on or before', msAdmitSaveSettings: 'Save settings', msAdmitSettingsSaved: 'Settings saved.',
    msAdmitErr_no_admit_cards: 'No card to print: every student here is held for dues.', msAdmitErr_admit_card_held: 'Your admit card is held until your dues are paid. Please contact the school office.',
    msAdmitErr_documents_unavailable: 'Admit cards cannot be made right now. Please try again later.', msAdmitErrGeneric: 'The admit cards could not be prepared.',

    msMyAdmitTitle: 'My admit cards', msMyAdmitNone: 'Your admit card appears here once the exam routine is published.', msMyAdmitHeld: 'Held until your dues are paid. Please contact the school office.',
    msDownloadEnglish: 'Download (English)', msDownloadBangla: 'Download (বাংলা)',

    msVerifyTitle: 'Admit card check', msVerifyCurrent: 'Valid admit card', msVerifyCurrentHint: 'It was printed from the routine now in force.', msVerifySuperseded: 'The routine has changed',
    msVerifySupersededHint: 'The routine was amended after this card was printed. Ask the school for a new card.', msVerifyNotFound: 'This admit card could not be found.',
    msVerifyChecking: 'Checking the admit card…', msVerifyTerm: 'Examination', msVerifyClass: 'Class and section', msVerifyVersion: 'Routine version', msVerifyIssued: 'Printed', msVerifyPrivacy: 'This check shows no name or identifier.',

    msMarksheetPdf: 'Marksheet (PDF)', msMarksheetLater: 'The printable marksheet is ready once this term’s result is published.',
  },
  bn: {
    msNavComments: 'শ্রেণি শিক্ষকের মন্তব্য', msNavAdmit: 'প্রবেশপত্র',
    msLoading: 'লোড হচ্ছে…', msLoadError: 'লোড করা যায়নি। আবার চেষ্টা করুন।', msErrGeneric: 'পরিবর্তন সংরক্ষণ করা যায়নি।', msErrForbidden: 'আপনি এই পরিবর্তন করতে পারবেন না।',
    msSave: 'সংরক্ষণ', msSaved: 'সংরক্ষিত হয়েছে।', msChoose: 'বেছে নিন…', msEnglish: 'English', msBangla: 'বাংলা',

    msCommentsTitle: 'শ্রেণি শিক্ষকের মন্তব্য',
    msCommentsIntro: 'প্রত্যেক শিক্ষার্থীর ফলাফল পত্রের জন্য একটি ছোট মন্তব্য লিখুন: তৈরি মন্তব্য বেছে নিন বা নিজে লিখুন। প্রধান শিক্ষক সেগুলো দেখে নেন। মন্তব্য কখনো প্রকাশিত ফলাফল বদলায় না; পরের ফলাফল পত্রে তা দেখা যায়।',
    msCommentsIntroReviewer: 'শ্রেণি শিক্ষকেরা প্রত্যেক শিক্ষার্থীর ফলাফল পত্রে যা লিখেছেন তা দেখে নিন। আপনি মন্তব্য সংশোধন করতে পারেন; আপনার সংশোধন পর্যালোচিত হিসেবে গণ্য হয়।',
    msSection: 'শাখা', msResult: 'ফলাফল', msAnnualOf: 'বার্ষিক ফলাফল {{session}}', msClassTeacherLine: 'শ্রেণি শিক্ষক: {{name}}', msNoClassTeacher: 'শ্রেণি শিক্ষক নির্ধারিত হয়নি',
    msNoSections: 'আপনি এখনো কোনো শাখার শ্রেণি শিক্ষক নন।', msNoSectionsHint: 'প্রশাসক শাখা পাতায় শ্রেণি শিক্ষক নির্ধারণ করেন।',
    msStudent: 'শিক্ষার্থী', msRoll: 'রোল', msComment: 'মন্তব্য', msStatus: 'অবস্থা', msGpa: 'জিপিএ',
    msNoComment: 'মন্তব্য নেই', msOwnWords: 'নিজের ভাষায়…', msOwnPlaceholder: 'ছোট মন্তব্য, ৪০০ অক্ষর পর্যন্ত', msCommentFor: '{{name}}-এর মন্তব্য', msOwnFor: '{{name}}-এর জন্য নিজের ভাষায় মন্তব্য',
    msCleared: 'মন্তব্য মুছে ফেলা হয়েছে।',
    msStatus_written: 'লেখা হয়েছে', msStatus_reviewed: 'পর্যালোচিত', msStatus_none: 'লেখা হয়নি', msWrittenBy: 'লিখেছেন {{name}}',
    msReviewAll: 'লেখা মন্তব্যগুলো পর্যালোচিত চিহ্নিত করুন', msReviewedCount: '{{n}}টি মন্তব্য পর্যালোচিত চিহ্নিত হয়েছে।', msNothingToReview: 'পর্যালোচনার অপেক্ষায় কিছু নেই।',
    msPresetsTitle: 'তৈরি মন্তব্য', msPresetsIntro: 'শ্রেণি শিক্ষকেরা এখান থেকে বেছে নেন। কোনো তৈরি মন্তব্য বদলালে এখন থেকে তৈরি প্রতিটি ফলাফল পত্রে নতুন লেখা ছাপা হয়।',
    msPresetEN: 'ইংরেজি', msPresetBN: 'বাংলা', msPresetOrder: 'ক্রম', msPresetActive: 'ব্যবহারে আছে', msPresetAdd: 'তৈরি মন্তব্য যোগ করুন', msPresetSave: 'মন্তব্য সংরক্ষণ', msPresetNew: 'নতুন তৈরি মন্তব্য',

    msClassTeachersTitle: 'শ্রেণি শিক্ষক', msClassTeachersIntro: 'প্রতি শাখায় একজন শ্রেণি শিক্ষক। তিনি তাঁর শিক্ষার্থীদের ফলাফল পত্রে মন্তব্য লেখেন ও প্রবেশপত্র ছাপেন; ফলাফল পত্রে তাঁর স্বাক্ষর ছাপা হয়।',
    msSession: 'শিক্ষাবর্ষ', msTeacher: 'শ্রেণি শিক্ষক', msNoTeacher: 'নির্ধারিত হয়নি', msSignatureMissing: 'এখনো স্বাক্ষর আপলোড হয়নি', msSignatureReady: 'স্বাক্ষর আপলোড হয়েছে', msAssigned: 'শ্রেণি শিক্ষক সংরক্ষিত হয়েছে।',
    msTeacherFor: '{{section}}-এর শ্রেণি শিক্ষক',

    msSignatureTitle: 'আমার স্বাক্ষর', msSignatureIntro: 'আপনি যেসব শাখার শ্রেণি শিক্ষক, সেগুলোর ফলাফল পত্রে এটি ছাপা হয়। স্বচ্ছ পটভূমির PNG, অথবা সাদা কাগজে আপনার স্বাক্ষরের পরিষ্কার ছবি বা স্ক্যান আপলোড করুন: কাগজ বাদ দেওয়া হয় এবং ছবি ছেঁটে ছোট করা হয়।',
    msSignatureUpload: 'স্বাক্ষর আপলোড', msSignatureReplace: 'স্বাক্ষর বদলান', msSignatureRemove: 'মুছুন', msSignatureNone: 'এখনো স্বাক্ষর নেই।', msSignatureSaved: 'স্বাক্ষর সংরক্ষিত হয়েছে।', msSignatureRemoved: 'স্বাক্ষর মুছে ফেলা হয়েছে।',
    msSignatureInvalid: 'স্পষ্ট স্বাক্ষরসহ PNG বা JPEG ছবি দিন (৪ মেগাবাইট পর্যন্ত)।', msSignatureAlt: 'আপনার স্বাক্ষর, যেভাবে ছাপা হয়',

    msAdmitTitle: 'প্রবেশপত্র', msAdmitIntro: 'প্রতিটি সাময়িকের প্রকাশিত পরীক্ষার রুটিন থেকে প্রবেশপত্র তৈরি হয়: প্রত্যেক শিক্ষার্থীর নিজের পরীক্ষা, কক্ষ ও আসন, নিয়মাবলি ও প্রধান শিক্ষকের স্বাক্ষরসহ। প্রতি A4 পাতায় দুটি প্রবেশপত্র।',
    msAdmitTerm: 'পরীক্ষা', msAdmitNoTerms: 'এখনো কোনো পরীক্ষার রুটিন প্রকাশিত হয়নি।', msAdmitNoTermsHint: 'কোনো সাময়িকের রুটিন প্রকাশিত হলে প্রবেশপত্র এখানে আসবে।', msAdmitVersion: 'রুটিন সংস্করণ {{n}}',
    msAdmitClass: 'শ্রেণি', msAdmitStudents: 'শিক্ষার্থী', msAdmitHeldCol: 'আটকে আছে', msAdmitPrintSection: '{{class}} {{section}} ছাপুন — {{language}}', msAdmitWholeClass: 'পুরো শ্রেণি {{class}}',
    msAdmitPreparing: 'প্রবেশপত্র তৈরি হচ্ছে…', msAdmitHeldOut: 'ডাউনলোড হয়েছে। বকেয়ার জন্য আটকে থাকা {{n}} জন বাদ পড়েছে।', msAdmitDone: 'ডাউনলোড হয়েছে।',
    msAdmitHeldTitle: 'বকেয়ার জন্য আটকে আছে', msAdmitHeldIntro: 'বকেয়ার শর্ত চালু: {{date}} বা তার আগের কোনো বকেয়া অপরিশোধিত থাকলে অফিস ছাড় না দেওয়া পর্যন্ত প্রবেশপত্র দেওয়া হবে না।',
    msAdmitHeldOff: 'বকেয়ার শর্ত বন্ধ: সবাই প্রবেশপত্র পাবে।', msAdmitNoHeld: 'নির্ধারিত তারিখ পর্যন্ত কারও বকেয়া নেই।',
    msAdmitDue: 'বকেয়া', msAdmitOldest: 'সবচেয়ে পুরোনো বকেয়া', msAdmitRelease: 'ছাড় দিন', msAdmitReleasedState: 'ছাড় দেওয়া হয়েছে', msAdmitHeldState: 'আটকে আছে', msAdmitHoldAgain: 'আবার আটকান', msAdmitNote: 'নোট (ঐচ্ছিক)', msAdmitNoteFor: '{{name}}-এর ছাড়ের নোট',
    msAdmitReleasedMsg: 'ছাড় দেওয়া হয়েছে: {{name}} প্রবেশপত্র পাবে।', msAdmitHeldMsg: '{{name}} আবার আটকে গেছে।',
    msAdmitSettingsTitle: 'নিয়মাবলি ও বকেয়ার শর্ত', msAdmitRulesEN: 'ইংরেজিতে নিয়মাবলি, প্রতি লাইনে একটি', msAdmitRulesBN: 'বাংলায় নিয়মাবলি, প্রতি লাইনে একটি', msAdmitRulesHint: 'খালি রাখলে ভেতরে দেওয়া নিয়মাবলি ছাপা হবে (ঘরে হালকা লেখায় দেখানো)।',
    msAdmitHold: 'বকেয়া থাকলে প্রবেশপত্র আটকে রাখুন', msAdmitHoldHint: 'শুরুতে বন্ধ। অফিস আলাদাভাবে শিক্ষার্থীকে ছাড় দিতে পারে।', msAdmitCutoff: 'যে তারিখ পর্যন্ত বকেয়া', msAdmitSaveSettings: 'সেটিংস সংরক্ষণ', msAdmitSettingsSaved: 'সেটিংস সংরক্ষিত হয়েছে।',
    msAdmitErr_no_admit_cards: 'ছাপার মতো প্রবেশপত্র নেই: এখানে সবাই বকেয়ার জন্য আটকে আছে।', msAdmitErr_admit_card_held: 'বকেয়া পরিশোধ না হওয়া পর্যন্ত আপনার প্রবেশপত্র আটকে আছে। অনুগ্রহ করে বিদ্যালয়ের অফিসে যোগাযোগ করুন।',
    msAdmitErr_documents_unavailable: 'এখন প্রবেশপত্র তৈরি করা যাচ্ছে না। পরে আবার চেষ্টা করুন।', msAdmitErrGeneric: 'প্রবেশপত্র তৈরি করা যায়নি।',

    msMyAdmitTitle: 'আমার প্রবেশপত্র', msMyAdmitNone: 'পরীক্ষার রুটিন প্রকাশিত হলে আপনার প্রবেশপত্র এখানে আসবে।', msMyAdmitHeld: 'বকেয়া পরিশোধ না হওয়া পর্যন্ত আটকে আছে। অনুগ্রহ করে বিদ্যালয়ের অফিসে যোগাযোগ করুন।',
    msDownloadEnglish: 'ডাউনলোড (English)', msDownloadBangla: 'ডাউনলোড (বাংলা)',

    msVerifyTitle: 'প্রবেশপত্র যাচাই', msVerifyCurrent: 'বৈধ প্রবেশপত্র', msVerifyCurrentHint: 'বর্তমানে কার্যকর রুটিন থেকে ছাপা হয়েছে।', msVerifySuperseded: 'রুটিন বদলেছে',
    msVerifySupersededHint: 'এই প্রবেশপত্র ছাপার পরে রুটিন সংশোধন করা হয়েছে। বিদ্যালয় থেকে নতুন প্রবেশপত্র নিন।', msVerifyNotFound: 'এই প্রবেশপত্র পাওয়া যায়নি।',
    msVerifyChecking: 'প্রবেশপত্র যাচাই হচ্ছে…', msVerifyTerm: 'পরীক্ষা', msVerifyClass: 'শ্রেণি ও শাখা', msVerifyVersion: 'রুটিন সংস্করণ', msVerifyIssued: 'ছাপা হয়েছে', msVerifyPrivacy: 'এই যাচাইয়ে কোনো নাম বা পরিচয় নম্বর দেখানো হয় না।',

    msMarksheetPdf: 'ফলাফল পত্র (PDF)', msMarksheetLater: 'এই সাময়িকের ফলাফল প্রকাশিত হলে ছাপার উপযোগী ফলাফল পত্র পাওয়া যাবে।',
  },
}
