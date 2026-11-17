package schoolsetup

// These are deliberately assembled TEST identities, not verified people.
// The paired spellings are curated together so a name is never translated
// mechanically. Personal and family information is left for school records.
type syntheticNamePart struct{ en, bn string }

var syntheticMaleNames = []syntheticNamePart{
	{"Arif", "আরিফ"}, {"Tanvir", "তানভীর"}, {"Mahin", "মাহিন"},
	{"Rafi", "রাফি"}, {"Samiul", "সামিউল"}, {"Farhan", "ফারহান"},
	{"Imran", "ইমরান"}, {"Nafis", "নাফিস"}, {"Rakib", "রাকিব"},
	{"Shakib", "সাকিব"}, {"Fardin", "ফারদিন"}, {"Ayman", "আয়মান"},
}

var syntheticFemaleNames = []syntheticNamePart{
	{"Ayesha", "আয়েশা"}, {"Nusrat", "নুসরাত"}, {"Sadia", "সাদিয়া"},
	{"Maliha", "মালিহা"}, {"Sumaiya", "সুমাইয়া"}, {"Samira", "সামিরা"},
	{"Fariha", "ফারিহা"}, {"Nafisa", "নাফিসা"}, {"Tamanna", "তামান্না"},
	{"Tania", "তানিয়া"}, {"Lamia", "লামিয়া"}, {"Tasnim", "তাসনিম"},
}

var syntheticSurnames = []syntheticNamePart{
	{"Rahman", "রহমান"}, {"Islam", "ইসলাম"}, {"Ahmed", "আহমেদ"},
	{"Hossain", "হোসেন"}, {"Chowdhury", "চৌধুরী"}, {"Karim", "করিম"},
	{"Alam", "আলম"}, {"Khan", "খান"}, {"Sarker", "সরকার"},
	{"Kabir", "কবির"},
}

// A batch's stable candidate order selects a different first/surname pair
// for its first 240 students. Repetition after that is permitted; admission
// numbers and login IDs, rather than display names, are unique identifiers.
func syntheticStudentName(index int) (en, bn, gender string) {
	gender = "male"
	given := syntheticMaleNames
	if index%2 == 1 {
		gender = "female"
		given = syntheticFemaleNames
	}
	position := index / 2
	first := given[position%len(given)]
	last := syntheticSurnames[(position/len(given))%len(syntheticSurnames)]
	return "TEST " + first.en + " " + last.en, "টেস্ট " + first.bn + " " + last.bn, gender
}
