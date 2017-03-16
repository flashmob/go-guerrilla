package response

import (
	"fmt"
	"math/rand"
	"time"
)

// This is an easter egg

const CRLF = "\r\n"

var quotes = struct {
	m map[int]string
}{m: map[int]string{
	0: "214 Maude Lebowski: He's a good man....and thorough.",
	1: "214 The Dude: I had a rough night and I hate the f***ing Eagles, man.",
	2: "214 Walter Sobchak: The chinaman is not the issue here... also dude, Asian American please",
	3: "214 The Dude: Walter, the chinamen who peed on my rug I can't give him a bill, so what the f**k are you talking about?",
	4: "214 The Dude: Hey, I know that guy, he's a nihilist. Karl Hungus.",
	5: "214-Malibu Police Chief: Mr. Treehorn tells us that he had to eject you from his garden party; that you were drunk and abusive." + CRLF +
		"214 The Dude: Mr. Treehorn treats objects like women, man.",
	6: "214 Walter Sobchak: Shut the f**k up, Donny!",
	7: "214-Donny: Shut the f**k up, Donny!" + CRLF +
		"214 Walter Sobchak: Shut the f**k up, Donny!",
	8:  "214 The Dude: It really tied the room together.",
	9:  "214 Walter Sobchak: Is this your homework, Larry?",
	10: "214 The Dude: Who the f**k are the Knutsens?",
	11: "214 The Dude: Yeah,well, that's just, like, your opinion, man.",
	12: "214-Walter Sobchak: Am I the only one who gives a s**t about the rules?!" + CRLF +
		"214 Walter Sobchak: Am I the only one who gives a s**t about the rules?",
	13: "214-Walter Sobchak: Am I wrong?" + CRLF +
		"214-The Dude: No, you're not wrong Walter, you're just an ass-hole." +
		"214 Walter Sobchak: Okay then.",
	14: "214-Private Snoop: you see what happens lebowski?" + CRLF +
		"214-The Dude: nobody calls me lebowski, you got the wrong guy, I'm the dude, man." + CRLF +
		"214-Private Snoop: Your name's Lebowski, Lebowski. Your wife is Bunny." + CRLF +
		"214-The Dude: My wife? Bunny? Do you see a wedding ring on my finger? " + CRLF +
		"214 Does this place look like I'm f**kin married? The toilet seat's up man!",
	15: "214-The Dude: Yeah man. it really tied the room together." + CRLF +
		"214-Donny: What tied the room together dude?" + CRLF +
		"214-The Dude: My rug." + CRLF +
		"214-Walter Sobchak: Were you listening to the Dude's story, Donny?" + CRLF +
		"214-Donny: I was bowling." + CRLF +
		"214-Walter Sobchak: So then you have no frame of reference here, Donny, " + CRLF +
		"214 You're like a child who wonders in the middle of movie.",
	16: "214-The Dude: She probably kidnapped herself." + CRLF +
		"214-Donny: What do you mean dude?" + CRLF +
		"214-The Dude: Rug Peers did not do this. look at it. " + CRLF +
		"214-A young trophy wife, marries this guy for his money, she figures he " + CRLF +
		"214-hasn't given her enough, she owes money all over town." + CRLF +
		"214 Walter Sobchak: That f**kin bitch.",
	17: "214 Walter Sobchak: Forget it, Donny, you're out of your element!",
	18: "214-Walter Sobchak: You want a toe? I can get you a toe, believe me." + CRLF +
		"214-There are ways, Dude. You don't wanna know about it, believe me. " + CRLF +
		"214-The Dude: Yeah, but Walter." +
		"214 Walter Sobchak: Hell, I can get you a toe by 3 o'clock this afternoon with nail polish.",
	19: "214 Walter Sobchak: Calmer then you are.",
	20: "214 Walter Sobchak: You are entering a world of pain",
	21: "214 The Dude: This aggression will not stand man.",
	22: "214 The Dude: His dudeness, duder, or el dudorino",
	23: "214 Walter Sobchak: Has the whole world gone crazy!",
	24: "214 Walter Sobchak: Calm down your being very undude.",
	25: "214-Donny: Are these the Nazis, Walter?" + CRLF +
		"214 Walter Sobchak: No Donny, these men are nihilists. There's nothing to be afraid of.",
	26: "214 Walter Sobchak: Well, it was parked in the handicapped zone. Perhaps they towed it.",
	27: "214-Private Snoop: I'm a brother shamus!" + CRLF +
		"214 The Dude: Brother Seamus? Like an Irish monk?",
	28: "214 Walter Sobchak: Have you ever of Vietnam? You're about to enter a world of pain!",
	29: "214-Donny: What's a pederast, Walter?" + CRLF +
		"214 Walter Sobchak: Shut the f**k up, Donny.",
	30: "214 The Dude: Hey, careful, man, there's a beverage here!",
	31: "214 The Stranger: Sometimes you eat the bar and sometimes, well, the bar eats you.",
	32: "214 Walter Sobchak: Goodnight, sweet prince.",
	33: "214 Jackie Treehorn: People forget the brain is the biggest erogenous zone.",
	34: "214-The Big Lebowski: What makes a man? Is it doing the right thing?" + CRLF +
		"214 The Dude: Sure, that and a pair of testicles.",
	35: "214 The Dude: At least I'm housebroken.",
	36: "214-Walter Sobchak: Three thousand years of beautiful tradition, from Moses to Sandy Koufax." + CRLF +
		"214 You're goddamn right I'm living in the f**king past!",
	37: "214-The Stranger: There's just one thing, dude." + CRLF +
		"214-The Dude: What's that?" + CRLF +
		"214-The Stranger: Do you have to use so many cuss words?" + CRLF +
		"214 The Dude: What the f**k you talkin' about?",
	38: "214-Walter Sobchak: I mean, say what you want about the tenets of National Socialism, " + CRLF +
		"214 Dude, at least it's an ethos.",
	39: "214 The Dude: My only hope is that the Big Lebowski kills me before the Germans can cut my d**k off.",
	40: "214 The Dude: You human paraquat!",
	41: "214 The Dude: Strikes and gutters, ups and downs.",
	42: "214 The Dude: Sooner or later you are going to have to face the fact that your a moron.",
	43: "214-The Dude: The fixes the cable?" + CRLF +
		"214 Maude Lebowski: Don't be fatuous Jerry.",
	44: "214 The Dude: Yeah, well, that's just, like, your opinion, man.",
	45: "214 The Dude: I don't need your sympathy, I need my Johnson.",
	46: "214 Donny: I am the walrus.",
	47: "214 The Dude: We f**ked it up!",
	48: "214 Jesus Quintana: You got that right, NO ONE f**ks with the jesus.",
	49: "214 Walter Sobchak: You can say what you want about the tenets of national socialism but at least it's an ethos.",
	50: "214-Walter Sobchak: f**king Germans. Nothing changes. f**king Nazis." + CRLF +
		"214-Donny: They were Nazis, Dude?" + CRLF +
		"214 Walter Sobchak: Oh, come on Donny, they were threatening castration! Are we gonna split hairs here? Am I wrong?",
	51: "214 Walter Sobchak: [pulls out a gun] Smokey, my friend, you are entering a world of pain.",
	52: "214 Walter Sobchak: This is what happens when you f**k a stranger in the ass!",
	53: "214-The Dude: We dropped off the money." + CRLF +
		"214-The Big Lebowski: *We*!?" + CRLF +
		"214 The Dude: *I*; the royal we.",
	54: "214 Walter Sobchak: You see what happens larry when you f**k a stranger in the ass.",
	55: "214 The Dude: The Dude abides.",
	56: "214 Walter Sobchak: f**k it dude, lets go bowling.",
	57: "214 The Dude: I can't be worrying about that s**t. Life goes on, man.",
	58: "214 Walter Sobchak: The ringer cannot look empty.",
	59: "214-Malibu Police Chief: I don't like your jerk-off name, I don't like your jerk-off face," + CRLF +
		"214 I don't like your j3rk-off behavior, and I don't like you... j3rk-off.",
	60: "214-Walter Sobchak: Has the whole world gone CRAZY? Am I the only one around here who gives" + CRLF +
		"214 a s**t about the rules? You think I'm f**kin' around, MARK IT ZERO!",
	61: "214 Walter Sobchak: Look, Larry. Have you ever heard of Vietnam?",
	62: "214 The Dude: Ha hey, this is a private residence man.",
	63: "214 The Dude: Obviously you're not a golfer.",
	64: "214 Walter Sobchak: You know, Dude, I myself dabbled in pacifism once. Not in Nam, of course.",
	65: "214 Walter Sobchak: Donny, you're out of your element!",
	66: "214 The Dude: Another caucasian, Gary.",
	67: "214-Bunny Lebowski: I'll s**k your c**k for a thousand dollars." + CRLF +
		"214-Brandt: Ah... Ha... ha... HA! Yes, we're all very fond of her." + CRLF +
		"214-Bunny Lebowski: Brandt can't watch, though. Or it's an extra hundred." + CRLF +
		"214 The Dude: Okay... just give me a minute. I gotta go find a cash machine...",
	68: "214 Nihilist: Ve vont ze mawney Lebowski!",
	69: "214 Walter Sobchak: Eight-year-olds, dude.",
	70: "214-The Dude: They peed on my rug, man!" + CRLF +
		"214-Walter Sobchak: f**king Nazis." + CRLF +
		"214-Donny: I don't know if they were Nazis, Walter..." + CRLF +
		"214 Walter Sobchak: Shut the f**k up, Donny. They were threatening castration!",
	71: "214 Jesus Quintana: I don't f**king care, it don't matter to Jesus.",
	72: "214-The Dude: Where's my car?" + CRLF +
		"214 Walter Sobchak: It was parked in a handicap zone, perhaps they towed it.",
	73: "214-Bunny Lebowski: Uli doesn't care about anything. He's a Nihilist!" + CRLF +
		"214 The Dude: Ah, that must be exhausting!",
	74: "214 Walter Sobchak: Smoky this is not Nam this is Bowling there are rules.",
	75: "214 Maude Lebowski: Vagina.",
	76: "214-Jesus Quintana: Let me tell you something pendejo. You pull any of your crazy s**t with us." + CRLF +
		"214-You flash your piece out on the lanes. I'll take it away from you and stick up your alps" + CRLF +
		"214-and pull the f**king trigger 'til it goes click." + CRLF +
		"214-The Dude: ...Jesus" + CRLF +
		"214 Jesus Quintana: You said it man, nobody f**ks with the Jesus.",
	77: "214-The Dude: You brought a f**king Pomeranian bowling?" + CRLF +
		"214-Walter Sobchak: Bought it bowling? I didn't rent it shoes. " + CRLF +
		"214 I'm not buying it a f**king beer. It's not taking your f**king turn, Dude.",
	78: "214 Walter Sobchak: Mark it as a zero.",
	79: "214-The Stranger: The Dude abides. I don't know about you, but I take comfort in that. " + CRLF +
		"214 It's good knowing he's out there, the Dude, takin' 'er easy for all us sinners.",
	80: "214 Walter Sobchak: Aw, f**k it Dude. Let's go bowling.",
	81: "214 Walter Sobchak: Life does not stop and start at your convenience you miserable piece of s**t.",
	82: "214 Walter Sobchak: I told that kraut a f**king thousand times that I don't roll on Shabbos!",
	83: "214 Walter Sobchak: This is what happens when you find a stranger in the alps!",
}}

// GetQuote returns a random quote from The big Lebowski
func GetQuote() string {
	rand.Seed(time.Now().UnixNano())
	return fmt.Sprintf("%s", quotes.m[rand.Intn(len(quotes.m))])
}
