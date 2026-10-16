import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import { schoolTranslations } from './schoolTranslations'
import { checkpoint4Translations } from './checkpoint4Translations'
import { academicTranslations } from './academicTranslations'
import { academicSessionsTranslations } from './academicSessionsTranslations'
import { sectionTranslations } from './sectionTranslations'
import { peopleTranslations } from './peopleTranslations'

const savedLanguage = localStorage.getItem('flybirds.language')
const language = savedLanguage === 'bn' ? 'bn' : 'en'
document.documentElement.lang = language

void i18n.use(initReactI18next).init({
  resources: {
    en: {
      translation: {
        appName: 'Fly_Birds',
        foundation: 'School management foundation',
        home: 'Home',
        language: 'Language',
        welcome: 'A better start for every school',
        description: 'The M0 foundation is ready for school configuration and role-based features in later milestones.',
        apiStatus: 'API status',
        checking: 'Checking…',
        online: 'Online',
        offline: 'Unavailable',
        healthNote: 'This checks the backend health endpoint. Database readiness is not included.',
        reload: 'Check again',
        notFound: 'Page not found',
        backHome: 'Back to home',
        signIn: 'Sign in',
        signingIn: 'Signing in…',
        signOut: 'Sign out',
        account: 'Account',
        accountHeading: 'Your account',
        accountDescription: 'Your current signed-in details are loaded from the API.',
        loadingAccount: 'Loading your account…',
        accountError: 'Your account could not be loaded. Try again.',
        role: 'Role',
        email: 'Email address',
        password: 'Password',
        welcomeBack: 'Welcome back',
        loginHeading: 'Sign in to your school workspace',
        loginDescription: 'Use the account issued by your school administrator.',
        invalidLoginForm: 'Enter a valid email and password of at most 72 bytes.',
        invalidCredentials: 'Invalid email or password.',
        authUnavailable: 'Sign-in is unavailable until the database is configured.',
        loginError: 'Sign-in failed. Please try again.',
        logoutError: 'Sign-out failed. Please try again.',
        sessionExpired: 'Your session expired. Please sign in again.',
        restoringSession: 'Restoring your session…',
        administration: 'Administration',
        permissionEditor: 'Permissions',
        permissionEditorDescription: 'Review and update the permissions assigned to each role. Changes take effect for active sessions immediately.',
        roles: 'Roles',
        permissionMatrix: 'Permission matrix',
        loadingPermissions: 'Loading permissions…',
        loadPermissionsError: 'Permissions could not be loaded.',
        permissionDenied: 'You do not have permission to view this page.',
        noRoles: 'No roles are available.',
        noPermissions: 'No permissions are available.',
        saveChanges: 'Save changes',
        saving: 'Saving…',
        permissionsSaved: 'Permissions saved.',
        savePermissionsError: 'Permissions could not be saved. Please try again.',
        lastAdminError: 'The last authorized administrator must remain.',
      },
    },
    bn: {
      translation: {
        appName: 'ফ্লাই বার্ডস',
        foundation: 'স্কুল ব্যবস্থাপনার ভিত্তি',
        home: 'হোম',
        language: 'ভাষা',
        welcome: 'প্রতিটি স্কুলের জন্য সুন্দর শুরু',
        description: 'পরবর্তী ধাপে স্কুলের সেটিংস ও ভূমিকা অনুযায়ী সুবিধা যোগ করার জন্য M0 ভিত্তি প্রস্তুত।',
        apiStatus: 'এপিআই অবস্থা',
        checking: 'যাচাই করা হচ্ছে…',
        online: 'সচল',
        offline: 'পাওয়া যাচ্ছে না',
        healthNote: 'এটি ব্যাকএন্ডের স্বাস্থ্য পরীক্ষা করে। ডাটাবেসের প্রস্তুতি এতে অন্তর্ভুক্ত নয়।',
        reload: 'আবার পরীক্ষা করুন',
        notFound: 'পৃষ্ঠাটি পাওয়া যায়নি',
        backHome: 'হোমে ফিরুন',
        signIn: 'সাইন ইন',
        signingIn: 'সাইন ইন করা হচ্ছে…',
        signOut: 'সাইন আউট',
        account: 'অ্যাকাউন্ট',
        accountHeading: 'আপনার অ্যাকাউন্ট',
        accountDescription: 'আপনার বর্তমান অ্যাকাউন্টের তথ্য এপিআই থেকে আনা হয়েছে।',
        loadingAccount: 'অ্যাকাউন্টের তথ্য আনা হচ্ছে…',
        accountError: 'অ্যাকাউন্টের তথ্য আনা যায়নি। আবার চেষ্টা করুন।',
        role: 'ভূমিকা',
        email: 'ইমেইল ঠিকানা',
        password: 'পাসওয়ার্ড',
        welcomeBack: 'আবার স্বাগতম',
        loginHeading: 'আপনার স্কুলের কর্মক্ষেত্রে সাইন ইন করুন',
        loginDescription: 'স্কুল প্রশাসকের দেওয়া অ্যাকাউন্ট ব্যবহার করুন।',
        invalidLoginForm: 'সঠিক ইমেইল এবং সর্বোচ্চ ৭২ বাইটের পাসওয়ার্ড লিখুন।',
        invalidCredentials: 'ইমেইল বা পাসওয়ার্ড সঠিক নয়।',
        authUnavailable: 'ডাটাবেস কনফিগার না হওয়া পর্যন্ত সাইন ইন করা যাবে না।',
        loginError: 'সাইন ইন ব্যর্থ হয়েছে। আবার চেষ্টা করুন।',
        logoutError: 'সাইন আউট ব্যর্থ হয়েছে। আবার চেষ্টা করুন।',
        sessionExpired: 'আপনার সেশনের মেয়াদ শেষ হয়েছে। আবার সাইন ইন করুন।',
        restoringSession: 'আপনার সেশন পুনরুদ্ধার করা হচ্ছে…',
        administration: 'প্রশাসন',
        permissionEditor: 'অনুমতি',
        permissionEditorDescription: 'প্রতিটি ভূমিকার অনুমতি দেখুন ও পরিবর্তন করুন। পরিবর্তন সক্রিয় সেশনেও সঙ্গে সঙ্গে কার্যকর হয়।',
        roles: 'ভূমিকা',
        permissionMatrix: 'অনুমতির তালিকা',
        loadingPermissions: 'অনুমতি আনা হচ্ছে…',
        loadPermissionsError: 'অনুমতি আনা যায়নি।',
        permissionDenied: 'এই পৃষ্ঠা দেখার অনুমতি আপনার নেই।',
        noRoles: 'কোনো ভূমিকা নেই।',
        noPermissions: 'কোনো অনুমতি নেই।',
        saveChanges: 'পরিবর্তন সংরক্ষণ করুন',
        saving: 'সংরক্ষণ করা হচ্ছে…',
        permissionsSaved: 'অনুমতি সংরক্ষণ করা হয়েছে।',
        savePermissionsError: 'অনুমতি সংরক্ষণ করা যায়নি। আবার চেষ্টা করুন।',
        lastAdminError: 'সর্বশেষ অনুমোদিত প্রশাসককে রাখতে হবে।',
      },
    },
  },
  lng: language,
  fallbackLng: 'en',
  interpolation: { escapeValue: false },
})

i18n.addResourceBundle('en', 'translation', schoolTranslations.en, true, true)
i18n.addResourceBundle('bn', 'translation', schoolTranslations.bn, true, true)
i18n.addResourceBundle('en', 'translation', checkpoint4Translations.en, true, true)
i18n.addResourceBundle('bn', 'translation', checkpoint4Translations.bn, true, true)
i18n.addResourceBundle('en', 'translation', academicTranslations.en, true, true)
i18n.addResourceBundle('bn', 'translation', academicTranslations.bn, true, true)
i18n.addResourceBundle('en', 'translation', academicSessionsTranslations.en, true, true)
i18n.addResourceBundle('bn', 'translation', academicSessionsTranslations.bn, true, true)
i18n.addResourceBundle('en', 'translation', sectionTranslations.en, true, true)
i18n.addResourceBundle('bn', 'translation', sectionTranslations.bn, true, true)
i18n.addResourceBundle('en', 'translation', peopleTranslations.en, true, true)
i18n.addResourceBundle('bn', 'translation', peopleTranslations.bn, true, true)

export default i18n
