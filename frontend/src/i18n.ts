import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

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
      },
    },
  },
  lng: language,
  fallbackLng: 'en',
  interpolation: { escapeValue: false },
})

export default i18n
