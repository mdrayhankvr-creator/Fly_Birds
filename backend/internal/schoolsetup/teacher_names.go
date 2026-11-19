package schoolsetup

// These are fictional TEST labels, not school-supplied staff identities.
var syntheticTeacherNames = [...]struct{ en, bn string }{
	{"Md. Afnan Rahman", "মো. আফনান রহমান"},
	{"Nusrat Jahan", "নুসরাত জাহান"},
	{"Md. Tanvir Hasan", "মো. তানভীর হাসান"},
	{"Farzana Akter", "ফারজানা আক্তার"},
	{"Md. Rafiq Mahmud", "মো. রফিক মাহমুদ"},
	{"Shamima Sultana", "শামীমা সুলতানা"},
	{"Md. Adnan Karim", "মো. আদনান করিম"},
	{"Sadia Islam", "সাদিয়া ইসলাম"},
	{"Md. Imran Hossain", "মো. ইমরান হোসেন"},
	{"Tahmina Yasmin", "তাহমিনা ইয়াসমিন"},
	{"Md. Samiul Haque", "মো. সামিউল হক"},
	{"Rumana Parvin", "রুমানা পারভীন"},
	{"Md. Fahim Chowdhury", "মো. ফাহিম চৌধুরী"},
	{"Sharmin Nahar", "শারমিন নাহার"},
	{"Md. Nabil Ahmed", "মো. নাবিল আহমেদ"},
}

func syntheticTeacherName(slot int) (string, string) {
	name := syntheticTeacherNames[slot-1]
	return "TEST " + name.en, "টেস্ট " + name.bn
}
