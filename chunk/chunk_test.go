package chunk

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/flashmob/go-guerrilla/backends"
	"github.com/flashmob/go-guerrilla/chunk/transfer"
	"github.com/flashmob/go-guerrilla/mail"
)

func TestChunkedBytesBuffer(t *testing.T) {
	var in string

	var buf chunkingBuffer
	buf.CapTo(64)

	// the data to write is over-aligned
	in = `123456789012345678901234567890123456789012345678901234567890abcde12345678901234567890123456789012345678901234567890123456789abcdef` // len == 130
	i, _ := buf.Write([]byte(in[:]))
	if i != len(in) {
		t.Error("did not write", len(in), "bytes")
	}

	// the data to write is aligned
	var buf2 chunkingBuffer
	buf2.CapTo(64)
	in = `123456789012345678901234567890123456789012345678901234567890abcde12345678901234567890123456789012345678901234567890123456789abcd` // len == 128
	i, _ = buf2.Write([]byte(in[:]))
	if i != len(in) {
		t.Error("did not write", len(in), "bytes")
	}

	// the data to write is under-aligned
	var buf3 chunkingBuffer
	buf3.CapTo(64)
	in = `123456789012345678901234567890123456789012345678901234567890abcde12345678901234567890123456789012345678901234567890123456789ab` // len == 126
	i, _ = buf3.Write([]byte(in[:]))
	if i != len(in) {
		t.Error("did not write", len(in), "bytes")
	}

	// the data to write is smaller than the buffer
	var buf4 chunkingBuffer
	buf4.CapTo(64)
	in = `1234567890` // len == 10
	i, _ = buf4.Write([]byte(in[:]))
	if i != len(in) {
		t.Error("did not write", len(in), "bytes")
	}

	// what if the buffer already contains stuff before Write is called
	// and the buffer len is smaller than the len of the slice of bytes we pass it?
	var buf5 chunkingBuffer
	buf5.CapTo(5)
	buf5.buf = append(buf5.buf, []byte{'a', 'b', 'c'}...)
	in = `1234567890` // len == 10
	i, _ = buf5.Write([]byte(in[:]))
	if i != len(in) {
		t.Error("did not write", len(in), "bytes")
	}
}

var n1 = `From:  Al Gore <vice-president@whitehouse.gov>
To:  White House Transportation Coordinator <transport@whitehouse.gov>
Subject: [Fwd: Map of Argentina with Description]
MIME-Version: 1.0
DKIM-Signature: v=1; a=rsa-sha256; c=relaxed; s=ncr424; d=reliancegeneral.co.in;
 h=List-Unsubscribe:MIME-Version:From:To:Reply-To:Date:Subject:Content-Type:Content-Transfer-Encoding:Message-ID; i=prospects@prospects.reliancegeneral.co.in;
 bh=F4UQPGEkpmh54C7v3DL8mm2db1QhZU4gRHR1jDqffG8=;
 b=MVltcq6/I9b218a370fuNFLNinR9zQcdBSmzttFkZ7TvV2mOsGrzrwORT8PKYq4KNJNOLBahswXf
   GwaMjDKT/5TXzegdX/L3f/X4bMAEO1einn+nUkVGLK4zVQus+KGqm4oP7uVXjqp70PWXScyWWkbT
   1PGUwRfPd/HTJG5IUqs=
Content-Type: multipart/mixed;
 boundary="D7F------------D7FD5A0B8AB9C65CCDBFA872"

This is a multi-part message in MIME format.
--D7F------------D7FD5A0B8AB9C65CCDBFA872
Content-Type: text/plain; charset=utf8
Content-Transfer-Encoding: 7bit

Fred,

Fire up Air Force One!  We're going South!

Thanks,
Al
--D7F------------D7FD5A0B8AB9C65CCDBFA872
Content-Type: message/rfc822
Content-Transfer-Encoding: 7bit
Content-Disposition: inline

Return-Path: <president@whitehouse.gov>
Received: from mailhost.whitehouse.gov ([192.168.51.200])
 by heartbeat.whitehouse.gov (8.8.8/8.8.8) with ESMTP id SAA22453
 for <vice-president@heartbeat.whitehouse.gov>;
 Mon, 13 Aug 1998 l8:14:23 +1000
Received: from the_big_box.whitehouse.gov ([192.168.51.50])
 by mailhost.whitehouse.gov (8.8.8/8.8.7) with ESMTP id RAA20366
 for vice-president@whitehouse.gov; Mon, 13 Aug 1998 17:42:41 +1000
 Date: Mon, 13 Aug 1998 17:42:41 +1000
Message-Id: <199804130742.RAA20366@mai1host.whitehouse.gov>
From: Bill Clinton <president@whitehouse.gov>
To: A1 (The Enforcer) Gore <vice-president@whitehouse.gov>
Subject:  Map of Argentina with Description
MIME-Version: 1.0
Content-Type: multipart/mixed;
 boundary="DC8------------DC8638F443D87A7F0726DEF7"

This is a multi-part message in MIME format.
--DC8------------DC8638F443D87A7F0726DEF7
Content-Type: text/plain; charset=utf8
Content-Transfer-Encoding: 7bit

Hi A1,

I finally figured out this MIME thing.  Pretty cool.  I'll send you
some sax music in .au files next week!

Anyway, the attached image is really too small to get a good look at
Argentina.  Try this for a much better map:

http://www.1one1yp1anet.com/dest/sam/graphics/map-arg.htm

Then again, shouldn't the CIA have something like that?

Bill
--DC8------------DC8638F443D87A7F0726DEF7
Content-Type: image/png; name="three.png"
Content-Transfer-Encoding: 8bit

`

var email = `From:  Al Gore <vice-president@whitehouse.gov>
To:  White House Transportation Coordinator <transport@whitehouse.gov>
Subject: [Fwd: Map of Argentina with Description]
MIME-Version: 1.0
DKIM-Signature: v=1; a=rsa-sha256; c=relaxed; s=ncr424; d=reliancegeneral.co.in;
 h=List-Unsubscribe:MIME-Version:From:To:Reply-To:Date:Subject:Content-Type:Content-Transfer-Encoding:Message-ID; i=prospects@prospects.reliancegeneral.co.in;
 bh=F4UQPGEkpmh54C7v3DL8mm2db1QhZU4gRHR1jDqffG8=;
 b=MVltcq6/I9b218a370fuNFLNinR9zQcdBSmzttFkZ7TvV2mOsGrzrwORT8PKYq4KNJNOLBahswXf
   GwaMjDKT/5TXzegdX/L3f/X4bMAEO1einn+nUkVGLK4zVQus+KGqm4oP7uVXjqp70PWXScyWWkbT
   1PGUwRfPd/HTJG5IUqs=
Content-Type: multipart/mixed;
 boundary="D7F------------D7FD5A0B8AB9C65CCDBFA872"

This is a multi-part message in MIME format.
--D7F------------D7FD5A0B8AB9C65CCDBFA872
Content-Type: text/plain; charset=us-ascii
Content-Transfer-Encoding: 7bit

Fred,

Fire up Air Force One!  We're going South!

Thanks,
Al
--D7F------------D7FD5A0B8AB9C65CCDBFA872
Content-Type: message/rfc822
Content-Transfer-Encoding: 7bit
Content-Disposition: inline

Return-Path: <president@whitehouse.gov>
Received: from mailhost.whitehouse.gov ([192.168.51.200])
 by heartbeat.whitehouse.gov (8.8.8/8.8.8) with ESMTP id SAA22453
 for <vice-president@heartbeat.whitehouse.gov>;
 Mon, 13 Aug 1998 l8:14:23 +1000
Received: from the_big_box.whitehouse.gov ([192.168.51.50])
 by mailhost.whitehouse.gov (8.8.8/8.8.7) with ESMTP id RAA20366
 for vice-president@whitehouse.gov; Mon, 13 Aug 1998 17:42:41 +1000
 Date: Mon, 13 Aug 1998 17:42:41 +1000
Message-Id: <199804130742.RAA20366@mai1host.whitehouse.gov>
From: Bill Clinton <president@whitehouse.gov>
To: A1 (The Enforcer) Gore <vice-president@whitehouse.gov>
Subject:  Map of Argentina with Description
MIME-Version: 1.0
Content-Type: multipart/mixed;
 boundary="DC8------------DC8638F443D87A7F0726DEF7"

This is a multi-part message in MIME format.
--DC8------------DC8638F443D87A7F0726DEF7
Content-Type: text/plain; charset=us-ascii
Content-Transfer-Encoding: 7bit

Hi A1,

I finally figured out this MIME thing.  Pretty cool.  I'll send you
some sax music in .au files next week!

Anyway, the attached image is really too small to get a good look at
Argentina.  Try this for a much better map:

http://www.1one1yp1anet.com/dest/sam/graphics/map-arg.htm

Then again, shouldn't the CIA have something like that?

Bill
--DC8------------DC8638F443D87A7F0726DEF7
Content-Type: image/gif; name="three.gif"
Content-Transfer-Encoding: base64
Content-Disposition: attachment; filename="three.gif"

R0lGODlhEAAeAPAAAP///wAAACH5BAEAAAAALAAAAAAQAB4AAAIfhI+py+0Po5y0onCD3lzbD15K
R4ZmiAHk6p3uC8dWAQA7
--DC8------------DC8638F443D87A7F0726DEF7--

--D7F------------D7FD5A0B8AB9C65CCDBFA872--

`

var email2 = `Delivered-To: b@sharklasers.com
Received: from aaa.cn (aaa.cn  [220.178.145.250])
	by 163.com with SMTP id 41f596e02e4da6a74d878a630a7f175e@163.com;
	Tue, 17 Sep 2019 01:16:43 +0000
From: "=?utf-8?Q?=E6=B1=9F=E5=8D=97=E6=A2=A6=E6=96=AD=E6=A8=AA=E6=B1=9F=E6=B8=9A?=" <y@grr.la>
To: <b@163.com>
Subject: =?utf-8?Q?=E5=BA=94=E5=8A=9B=E6=AF=94b?=
Date: Tue, 17 Sep 2019 09:16:29 +0800
Message-ID: <KOHECNHGNFAINPPHKAKOOIAAHJKF.y@grr.la>
MIME-Version: 1.0
Content-Type: text/html;
	charset="utf-8"
Content-Transfer-Encoding: 8bit
X-Priority: 3 (Normal)
X-MSMail-Priority: Normal
X-Mailer: Microsoft Outlook IMO, Build 9.0.2416 (9.0.2911.0)
Importance: Normal
X-MimeOLE: Produced By Microsoft MimeOLE V6.00.2900.2180

<P><SPAN style='FONT-SIZE: large; FONT-FAMILY: "lucida Grande", Verdana, "Microsoft YaHei"; WHITE-SPACE: normal; WORD-SPACING: 0px; TEXT-TRANSFORM: none; FLOAT: none; FONT-WEIGHT: normal; COLOR: rgb(0,0,0); FONT-STYLE: normal; ORPHANS: 2; WIDOWS: 2; DISPLAY: inline !important; LETTER-SPACING: normal; BACKGROUND-COLOR: rgb(255,255,255); TEXT-INDENT: 0px; font-variant-ligatures: normal; font-variant-caps: normal; -webkit-text-stroke-width: 0px; text-decoration-style: initial; text-decoration-color: initial'><FONT size=2><FONT color=black><FONT color=#008040><FONT color=#000000><FONT color=silver>Date:2019-09-17<BR>Account:b@163.com</FONT><FONT size=3> </FONT></FONT>
<HR>

<P></P>
<P></FONT></FONT></FONT></SPAN><SPAN style='FONT-SIZE: large; FONT-FAMILY: "lucida Grande", Verdana, "Microsoft YaHei"; WHITE-SPACE: normal; WORD-SPACING: 0px; TEXT-TRANSFORM: none; FLOAT: none; FONT-WEIGHT: normal; COLOR: rgb(0,0,0); FONT-STYLE: normal; ORPHANS: 2; WIDOWS: 2; DISPLAY: inline !important; LETTER-SPACING: normal; BACKGROUND-COLOR: rgb(255,255,255); TEXT-INDENT: 0px; font-variant-ligatures: normal; font-variant-caps: normal; -webkit-text-stroke-width: 0px; text-decoration-style: initial; text-decoration-color: initial'><FONT color=black size=2>丅<span style="position:absolute; bottom:-3000px"><交通指挥灯></span>載<span style="position:absolute; bottom:-3000px"><硝化作用></span>а<span style="position:absolute; bottom:-3000px"><柿子></span>ρ<span style="position:absolute; bottom:-3000px"><竞争性领域></span>ρ<span style="position:absolute; bottom:-3000px"><毛虫></span>签<span style="position:absolute; bottom:-3000px"><非持久性病毒></span>到<span style="position:absolute; bottom:-3000px"><街灯></span>領<span style="position:absolute; bottom:-3000px"><麦蜘蛛></span>１<span style="position:absolute; bottom:-3000px"><求索></span>６<span style="position:absolute; bottom:-3000px"><企业标准></span>╇<span style="position:absolute; bottom:-3000px"><气候条件></span>，<span style="position:absolute; bottom:-3000px"><农村劳动力转移></span>登<span style="position:absolute; bottom:-3000px"><穴深></span>杁<span style="position:absolute; bottom:-3000px"><预备队></span>2<span style="position:absolute; bottom:-3000px"><农机具></span>Ч<span style="position:absolute; bottom:-3000px"><营养袋育苗></span>棋<span style="position:absolute; bottom:-3000px"><生物肥料></span>牌<span style="position:absolute; bottom:-3000px"><光照强度></span>2<span style="position:absolute; bottom:-3000px"><改良系谱法></span>４<span style="position:absolute; bottom:-3000px"><加大投入></span>9<span style="position:absolute; bottom:-3000px"><茶叶产业办公室></span>８<span style="position:absolute; bottom:-3000px"><省局></span>8<span style="position:absolute; bottom:-3000px"><辐照></span>４<span style="position:absolute; bottom:-3000px"><百亩></span>.<span style="position:absolute; bottom:-3000px"><上行列车></span>ｃ<span style="position:absolute; bottom:-3000px"><竹节></span>δ<span style="position:absolute; bottom:-3000px"><山嘴></span>м<span style="position:absolute; bottom:-3000px"><股所></span>，<span style="position:absolute; bottom:-3000px"><科学谋划></span>吋<span style="position:absolute; bottom:-3000px"><领海></span>款<span style="position:absolute; bottom:-3000px"><异常型种子></span>１<span style="position:absolute; bottom:-3000px"><董家村></span> <span style="position:absolute; bottom:-3000px"><平面球形图></span>０<span style="position:absolute; bottom:-3000px"><先来></span> <span style="position:absolute; bottom:-3000px"><藜叶斑病></span>０<span style="position:absolute; bottom:-3000px"><竞职></span>宋<span style="position:absolute; bottom:-3000px"><二棱大麦></span>２<span style="position:absolute; bottom:-3000px"><茎枯病></span> <span style="position:absolute; bottom:-3000px"><宁洱县></span>０<span style="position:absolute; bottom:-3000px"><共枕></span>鎵<span style="position:absolute; bottom:-3000px"><钙镁磷肥></span>專<span style="position:absolute; bottom:-3000px"><校正></span>員<span style="position:absolute; bottom:-3000px"><砍青></span>４<span style="position:absolute; bottom:-3000px"><独具特色></span>8<span style="position:absolute; bottom:-3000px"><装草机></span>６<span style="position:absolute; bottom:-3000px"><斑螟></span>1<span style="position:absolute; bottom:-3000px"><补偿机制></span>３<span style="position:absolute; bottom:-3000px"><创意策划></span>8<span style="position:absolute; bottom:-3000px"><投稿家></span>３<span style="position:absolute; bottom:-3000px"><茶点></span>2<span style="position:absolute; bottom:-3000px"><量天尺枯萎腐烂病></span>０<span style="position:absolute; bottom:-3000px"><河尾></span>翎<span style="position:absolute; bottom:-3000px"><台湾稻螟></span>。<span style="position:absolute; bottom:-3000px"><春城晚报></span></FONT></SPAN></P>
<P><SPAN style='FONT-SIZE: large; FONT-FAMILY: "lucida Grande", Verdana, "Microsoft YaHei"; WHITE-SPACE: normal; WORD-SPACING: 0px; TEXT-TRANSFORM: none; FLOAT: none; FONT-WEIGHT: normal; COLOR: rgb(0,0,0); FONT-STYLE: normal; ORPHANS: 2; WIDOWS: 2; DISPLAY: inline !important; LETTER-SPACING: normal; BACKGROUND-COLOR: rgb(255,255,255); TEXT-INDENT: 0px; font-variant-ligatures: normal; font-variant-caps: normal; -webkit-text-stroke-width: 0px; text-decoration-style: initial; text-decoration-color: initial'></SPAN><FONT color=silver size=2>Your words appear idle to me; give them proof, and I will listen.</FONT></P>`

var email3 = `Delivered-To: nevaeh@sharklasers.com
Received: from bb_dyn_pb-146-88-38-36.violin.co.th (bb_dyn_pb-146-88-38-36.violin.co.th  [146.88.38.36])
	by sharklasers.com with SMTP id d0e961595a207a79ab84603750372de8@sharklasers.com;
	Tue, 17 Sep 2019 01:13:00 +0000
Received: from mx03.listsystemsf.net [100.20.38.85] by mxs.perenter.com with SMTP; Tue, 17 Sep 2019 04:57:59 +0500
Received: from mts.locks.grgtween.net ([Tue, 17 Sep 2019 04:52:27 +0500])
	by webmail.halftomorrow.com with LOCAL; Tue, 17 Sep 2019 04:52:27 +0500
Received: from mail.naihautsui.co.kr ([61.220.30.1]) by mtu67.syds.piswix.net with ASMTP; Tue, 17 Sep 2019 04:47:25 +0500
Received: from unknown (HELO mx03.listsystemsf.net) (Tue, 17 Sep 2019 04:41:45 +0500)
	by smtp-server1.cfdenselr.com with LOCAL; Tue, 17 Sep 2019 04:41:45 +0500
Message-ID: <78431AF2.E9B20F56@violin.co.th>
Date: Tue, 17 Sep 2019 04:14:56 +0500
Reply-To: "Nevaeh" <JustinRichardson@violin.co.th>
From: "Nevaeh" <JustinRichardson@violin.co.th>
User-Agent: Mozilla 4.73 [de]C-CCK-MCD DT  (Win98; U)
X-Accept-Language: en-us
MIME-Version: 1.0
To: "Nevaeh" <nevaeh@sharklasers.com>
Subject: czy m�glbys spotkac sie ze mna w weekend?
Content-Type: text/html;
	charset="iso-8859-1""
Content-Transfer-Encoding: base64

PCFkb2N0eXBlIGh0bWw+DQo8aHRtbD4NCjxoZWFkPg0KPG1ldGEgY2hhcnNldD0idXRmLTgiPg0K
PC9oZWFkPg0KPGJvZHk+DQo8dGFibGUgd2lkdGg9IjYwMCIgYm9yZGVyPSIwIiBhbGlnbj0iY2Vu
dGVyIiBzdHlsZT0iZm9udC1mYW1pbHk6IEFyaWFsOyBmb250LXNpemU6IDE4cHgiPg0KPHRib2R5
Pg0KPHRyPg0KPHRoIGhlaWdodD0iNjAiIHNjb3BlPSJjb2wiPk5hamdvcmV0c3plIGR6aWV3Y3p5
bnkgaSBzYW1vdG5lIGtvYmlldHksIGt083JlIGNoY2Egc2Vrc3UuPG9sPjwvb2w+PC90aD4NCjwv
dHI+DQo8dGQgaGVpZ2h0PSIyMjMiIGFsaWduPSJjZW50ZXIiPjxwPk5hIG5hc3plaiBzdHJvbmll
IGdyb21hZHpvbmUgc2EgbWlsaW9ueSBwcm9maWxpIGtvYmlldC4gV3N6eXNjeSBjaGNhIHRlcmF6
IHBpZXByenljLjwvcD4NCjxoZWFkZXI+PC9oZWFkZXI+DQo8cD5OYSBwcnp5a2xhZCBzYSBXIFRX
T0lNIE1JRVNDSUUuIENoY2VzeiBpbm55Y2g/IFpuYWpkeiBuYSBuYXN6ZWogc3Ryb25pZSE8L3A+
DQo8dGFibGUgY2xhc3M9Im1jbkJ1dHRvbkNvbnRlbnRDb250YWluZXIiIHN0eWxlPSJib3JkZXIt
Y29sbGFwc2U6IHNlcGFyYXRlICEgaW1wb3J0YW50O2JvcmRlci1yYWRpdXM6IDNweDtiYWNrZ3Jv
dW5kLWNvbG9yOiAjRTc0MTQxOyIgYm9yZGVyPSIwIiBjZWxsc3BhY2luZz0iMCIgY2VsbHBhZGRp
bmc9IjAiPg0KIDx0Ym9keT4NCiA8dHI+DQogPHRkIGNsYXNzPSJtY25CdXR0b25Db250ZW50IiBz
dHlsZT0iZm9udC1mYW1pbHk6IEFyaWFsOyBmb250LXNpemU6IDIycHg7IHBhZGRpbmc6IDE1cHgg
MjVweDsiIHZhbGlnbj0ibWlkZGxlIiBhbGlnbj0iY2VudGVyIj4NCiA8YSBjbGFzcz0ibWNuQnV0
dG9uICIgaHJlZj0iaHR0cDovL2JldGhhbnkuc3UiIHRhcmdldD0iX2JsYW5rIiBzdHlsZT0iZm9u
dC13ZWlnaHQ6IG5vcm1hbDtsZXR0ZXItc3BhY2luZzogbm9ybWFsO2xpbmUtaGVpZ2h0OiAxMDAl
O3RleHQtYWxpZ246IGNlbnRlcjt0ZXh0LWRlY29yYXRpb246IG5vbmU7Y29sb3I6ICNGRkZGRkY7
Ij5odHRwOi8vYmV0aGFueS5zdTwvYT4NCiA8L3RkPg0KIDwvdHI+DQogPC90Ym9keT4NCiA8L3Rh
YmxlPjx0YWJsZSB3aWR0aD0iMjglIiBib3JkZXI9IjAiPjx0Ym9keT48dHI+PHRkPjwvdGQ+PHRk
PjwvdGQ+PHRkPjwvdGQ+PC90cj48L3Rib2R5PjwvdGFibGU+DQo8dGFibGUgc3R5bGU9Im1pbi13
aWR0aDoxMDAlOyIgY2xhc3M9Im1jblRleHRDb250ZW50Q29udGFpbmVyIiBhbGlnbj0ibGVmdCIg
Ym9yZGVyPSIwIiBjZWxscGFkZGluZz0iMCIgY2VsbHNwYWNpbmc9IjAiIHdpZHRoPSIxMDAlIj4N
Cjx0Ym9keT4NCiA8dHI+DQo8dGQgYWxpZ249ImNlbnRlciIgdmFsaWduPSJ0b3AiIGNsYXNzPSJt
Y25UZXh0Q29udGVudCIgc3R5bGU9InBhZGRpbmc6IDlweCAxOHB4O2NvbG9yOiAjNkI2QjZCO2Zv
bnQtZmFtaWx5OiBWZXJkYW5hLEdlbmV2YSxzYW5zLXNlcmlmO2ZvbnQtc2l6ZTogMTFweDsiPg0K
VXp5aiB0ZWdvIGxpbmt1LCBqZXNsaSBwcnp5Y2lzayBuaWUgZHppYWxhPGJyPg0KPGEgaHJlZj0i
aHR0cDovL2JldGhhbnkuc3UiIHRhcmdldD0iX2JsYW5rIj5odHRwOi8vYmV0aGFueS5zdTwvYT48
YnI+DQpTa29waXVqIGkgd2tsZWogbGluayBkbyBwcnplZ2xhZGFya2k8L3RkPg0KPC90cj4NCjwv
dGJvZHk+PC90YWJsZT48L3RkPg0KPC90cj4gDQo8L3Rib2R5Pg0KPC90YWJsZT4NCjxvbD48cD48
L3A+PC9vbD4NCjx0YWJsZSB3aWR0aD0iNjAwIiBib3JkZXI9IjAiIGFsaWduPSJjZW50ZXIiPg0K
IDx0Ym9keT4NCiA8dHI+DQogPHRkPjxhIGhyZWY9Imh0dHA6Ly9iZXRoYW55LnN1Ij48cCBzdHls
ZT0idGV4dC1hbGlnbjogY2VudGVyIj5DYW1pbGE8L3A+DQogPG5hdj48L25hdj4NCiA8dGFibGU+
DQogPHRyPg0KIDx0ZCB2YWxpZ249InRvcCIgc3R5bGU9ImJhY2tncm91bmQ6IHVybChodHRwczov
L3RoZWNoaXZlLmZpbGVzLndvcmRwcmVzcy5jb20vMjAxOS8wOS9iODE0NmEyOTI3ODY4ODkxNzk4
ODY1NDhlN2QzOWEzZV93aWR0aC02MDAuanBlZz9xdWFsaXR5PTEwMCZzdHJpcD1pbmZvJnc9NjQx
Jnpvb209Mikgbm8tcmVwZWF0IGNlbnRlcjtiYWNrZ3JvdW5kLXBvc2l0aW9uOiB0b3A7YmFja2dy
b3VuZC1zaXplOiBjb3ZlcjsiPjwhLS1baWYgZ3RlIG1zbyA5XT4gPHY6cmVjdCB4bWxuczp2PSJ1
cm46c2NoZW1hcy1taWNyb3NvZnQtY29tOnZtbCIgZmlsbD0idHJ1ZSIgc3Ryb2tlPSJmYWxzZSIg
c3R5bGU9Im1zby13aWR0aC1wZXJjZW50OjEwMDA7aGVpZ2h0OjQwMHB4OyI+IDx2OmZpbGwgdHlw
ZT0idGlsZSIgc3JjPSJodHRwczovL3RoZWNoaXZlLmZpbGVzLndvcmRwcmVzcy5jb20vMjAxOS8w
OC82YWU4NzFiNTlmYjUxMDc1ZGMwMzE3ZDBiOTkzZjJhOV93aWR0aC02MDAuanBnP3F1YWxpdHk9
MTAwJnN0cmlwPWluZm8mdz02NDEmem9vbT0yIiAvPiA8djp0ZXh0Ym94IGluc2V0PSIwLDAsMCww
Ij4gPCFbZW5kaWZdLS0+DQogPGRpdj4NCiA8Y2VudGVyPg0KIDx0YWJsZSBjZWxsc3BhY2luZz0i
MCIgY2VsbHBhZGRpbmc9IjAiIHdpZHRoPSIyODAiIGhlaWdodD0iNDAwIj4NCiA8dHI+DQogPHRk
IHZhbGlnbj0ibWlkZGxlIiBzdHlsZT0idmVydGljYWwtYWxpZ246bWlkZGxlO3RleHQtYWxpZ246
bGVmdDsiIGNsYXNzPSJtb2JpbGUtY2VudGVyIiBoZWlnaHQ9IjQwMCI+PGFydGljbGU+PC9hcnRp
Y2xlPiA8L3RkPg0KIDwvdHI+DQogPC90YWJsZT4NCiA8L2NlbnRlcj4NCiA8L2Rpdj4NCiA8IS0t
W2lmIGd0ZSBtc28gOV0+IDwvdjp0ZXh0Ym94PiA8L3Y6cmVjdD4gPCFbZW5kaWZdLS0+PC90ZD4N
CiA8L3RyPg0KIDwvdGFibGU+DQogPC9hPjwvdGQ+DQogPHRkPjxhIGhyZWY9Imh0dHA6Ly9iZXRo
YW55LnN1Ij48cCBzdHlsZT0idGV4dC1hbGlnbjogY2VudGVyIj5NaWxhPC9wPg0KIDxkaXY+PC9k
aXY+DQogPHRhYmxlPg0KIDx0cj4NCiA8dGQgdmFsaWduPSJ0b3AiIHN0eWxlPSJiYWNrZ3JvdW5k
OiB1cmwoaHR0cHM6Ly90aGVjaGl2ZS5maWxlcy53b3JkcHJlc3MuY29tLzIwMTkvMDgvODg1ZGFi
OTM2MGZiYzY2NGMzYTNhNDQwOGI1NTE2ZDUtMS5qcGc/cXVhbGl0eT0xMDAmc3RyaXA9aW5mbyZ3
PTY0MSZ6b29tPTIpIG5vLXJlcGVhdCBjZW50ZXI7YmFja2dyb3VuZC1wb3NpdGlvbjogdG9wO2Jh
Y2tncm91bmQtc2l6ZTogY292ZXI7Ij48IS0tW2lmIGd0ZSBtc28gOV0+IDx2OnJlY3QgeG1sbnM6
dj0idXJuOnNjaGVtYXMtbWljcm9zb2Z0LWNvbTp2bWwiIGZpbGw9InRydWUiIHN0cm9rZT0iZmFs
c2UiIHN0eWxlPSJtc28td2lkdGgtcGVyY2VudDoxMDAwO2hlaWdodDo0MDBweDsiPiA8djpmaWxs
IHR5cGU9InRpbGUiIHNyYz0iaHR0cHM6Ly90aGVjaGl2ZS5maWxlcy53b3JkcHJlc3MuY29tLzIw
MTkvMDgvMGE2Mzc2MDVkYzhkOTcyNzRhZWFkODVhOGY0YTJmYjkuanBnP3F1YWxpdHk9MTAwJnN0
cmlwPWluZm8mdz02MDAiIC8+IDx2OnRleHRib3ggaW5zZXQ9IjAsMCwwLDAiPiA8IVtlbmRpZl0t
LT4NCg0KIDxkaXY+DQogPGNlbnRlcj4NCiA8dGFibGUgY2VsbHNwYWNpbmc9IjAiIGNlbGxwYWRk
aW5nPSIwIiB3aWR0aD0iMjgwIiBoZWlnaHQ9IjQwMCI+DQogPHRyPg0KIDx0ZCB2YWxpZ249Im1p
ZGRsZSIgc3R5bGU9InZlcnRpY2FsLWFsaWduOm1pZGRsZTt0ZXh0LWFsaWduOmxlZnQ7IiBjbGFz
cz0ibW9iaWxlLWNlbnRlciIgaGVpZ2h0PSI0MDAiPjxocj4gPC90ZD4NCiA8L3RyPg0KIDwvdGFi
bGU+DQogPC9jZW50ZXI+DQogPC9kaXY+DQogPCEtLVtpZiBndGUgbXNvIDldPiA8L3Y6dGV4dGJv
eD4gPC92OnJlY3Q+IDwhW2VuZGlmXS0tPjwvdGQ+DQogPC90cj4NCiA8L3RhYmxlPg0KIDwvYT48
L3RkPg0KIDwvdHI+DQogPHRyPg0KIDx0ZD48YSBocmVmPSJodHRwOi8vYmV0aGFueS5zdSI+PHAg
c3R5bGU9InRleHQtYWxpZ246IGNlbnRlciI+THVuYTwvcD4NCiA8dGFibGUgd2lkdGg9Ijc0JSIg
Ym9yZGVyPSIwIj48dGJvZHk+PHRyPjx0ZD48L3RkPjx0ZD48L3RkPjx0ZD48L3RkPjx0ZD48L3Rk
Pjx0ZD48L3RkPjwvdHI+PC90Ym9keT48L3RhYmxlPg0KIDx0YWJsZT4NCiA8dHI+DQogPHRkIHZh
bGlnbj0idG9wIiBzdHlsZT0iYmFja2dyb3VuZDogdXJsKGh0dHBzOi8vdGhlY2hpdmUuZmlsZXMu
d29yZHByZXNzLmNvbS8yMDE5LzA4LzA2ZTU2YTU4ZjQ3ZDM0OGEyMjc3NmYyOTFlNjg2OWEwLTEu
anBnP3F1YWxpdHk9MTAwJnN0cmlwPWluZm8mdz02NDEmem9vbT0yKSBuby1yZXBlYXQgY2VudGVy
O2JhY2tncm91bmQtcG9zaXRpb246IHRvcDtiYWNrZ3JvdW5kLXNpemU6IGNvdmVyOyI+PCEtLVtp
ZiBndGUgbXNvIDldPiA8djpyZWN0IHhtbG5zOnY9InVybjpzY2hlbWFzLW1pY3Jvc29mdC1jb206
dm1sIiBmaWxsPSJ0cnVlIiBzdHJva2U9ImZhbHNlIiBzdHlsZT0ibXNvLXdpZHRoLXBlcmNlbnQ6
MTAwMDtoZWlnaHQ6NDAwcHg7Ij4gPHY6ZmlsbCB0eXBlPSJ0aWxlIiBzcmM9Imh0dHBzOi8vdGhl
Y2hpdmUuZmlsZXMud29yZHByZXNzLmNvbS8yMDE5LzA4LzhhYjRkYzcxMjFlYTVhMzdiMTc3NjNm
ZjRhNDA1MTVlLmpwZz9xdWFsaXR5PTEwMCZzdHJpcD1pbmZvJnc9NjQxJnpvb209MiIgLz4gPHY6
dGV4dGJveCBpbnNldD0iMCwwLDAsMCI+IDwhW2VuZGlmXS0tPg0KIDxkaXY+DQogPGNlbnRlcj4N
CiA8dGFibGUgY2VsbHNwYWNpbmc9IjAiIGNlbGxwYWRkaW5nPSIwIiB3aWR0aD0iMjgwIiBoZWln
aHQ9IjQwMCI+DQogPHRyPg0KIDx0ZCB2YWxpZ249Im1pZGRsZSIgc3R5bGU9InZlcnRpY2FsLWFs
aWduOm1pZGRsZTt0ZXh0LWFsaWduOmxlZnQ7IiBjbGFzcz0ibW9iaWxlLWNlbnRlciIgaGVpZ2h0
PSI0MDAiPjxicj4gPC90ZD4NCiA8L3RyPg0KIDwvdGFibGU+DQogPC9jZW50ZXI+DQogPC9kaXY+
DQogPCEtLVtpZiBndGUgbXNvIDldPiA8L3Y6dGV4dGJveD4gPC92OnJlY3Q+IDwhW2VuZGlmXS0t
PjwvdGQ+DQogPC90cj4NCiA8L3RhYmxlPg0KIDwvYT48L3RkPg0KIDx0ZD48YSBocmVmPSJodHRw
Oi8vYmV0aGFueS5zdSI+PHAgc3R5bGU9InRleHQtYWxpZ246IGNlbnRlciI+U2F2YW5uYWg8L3A+
DQogPG9sPjwvb2w+DQogPHRhYmxlPg0KIDx0cj4NCiA8dGQgdmFsaWduPSJ0b3AiIHN0eWxlPSJi
YWNrZ3JvdW5kOiB1cmwoaHR0cHM6Ly90aGVjaGl2ZS5maWxlcy53b3JkcHJlc3MuY29tLzIwMTkv
MDgvYzA4MzYxNTE2MzUxNDFkNDhlY2ZmYTNkYmZkOGYxZDYuanBnP3F1YWxpdHk9MTAwJnN0cmlw
PWluZm8mdz02NDEmem9vbT0yKSBuby1yZXBlYXQgY2VudGVyO2JhY2tncm91bmQtcG9zaXRpb246
IHRvcDtiYWNrZ3JvdW5kLXNpemU6IGNvdmVyOyI+PCEtLVtpZiBndGUgbXNvIDldPiA8djpyZWN0
IHhtbG5zOnY9InVybjpzY2hlbWFzLW1pY3Jvc29mdC1jb206dm1sIiBmaWxsPSJ0cnVlIiBzdHJv
a2U9ImZhbHNlIiBzdHlsZT0ibXNvLXdpZHRoLXBlcmNlbnQ6MTAwMDtoZWlnaHQ6NDAwcHg7Ij4g
PHY6ZmlsbCB0eXBlPSJ0aWxlIiBzcmM9Imh0dHBzOi8vdGhlY2hpdmUuZmlsZXMud29yZHByZXNz
LmNvbS8yMDE5LzA4L2NhMWI4MWI5MTkyYTZkMzEyNTI1MmYwYzIwZWIxMjVjLmpwZz9xdWFsaXR5
PTEwMCZzdHJpcD1pbmZvJnc9NjQxJnpvb209MiIgLz4gPHY6dGV4dGJveCBpbnNldD0iMCwwLDAs
MCI+IDwhW2VuZGlmXS0tPg0KIDxkaXY+DQogPGNlbnRlcj4NCiA8dGFibGUgY2VsbHNwYWNpbmc9
IjAiIGNlbGxwYWRkaW5nPSIwIiB3aWR0aD0iMjgwIiBoZWlnaHQ9IjQwMCI+DQogPHRyPg0KIDx0
ZCB2YWxpZ249Im1pZGRsZSIgc3R5bGU9InZlcnRpY2FsLWFsaWduOm1pZGRsZTt0ZXh0LWFsaWdu
OmxlZnQ7IiBjbGFzcz0ibW9iaWxlLWNlbnRlciIgaGVpZ2h0PSI0MDAiPjxtYWluPjwvbWFpbj4g
PC90ZD4NCiA8L3RyPg0KIDwvdGFibGU+DQogPC9jZW50ZXI+DQogPC9kaXY+DQogPCEtLVtpZiBn
dGUgbXNvIDldPiA8L3Y6dGV4dGJveD4gPC92OnJlY3Q+IDwhW2VuZGlmXS0tPjwvdGQ+DQogPC90
cj4NCiA8L3RhYmxlPg0KIDwvYT48L3RkPg0KIDwvdHI+DQogPC90Ym9keT4NCjwvdGFibGU+DQo8
dGFibGUgd2lkdGg9IjYxJSIgYm9yZGVyPSIwIj48dGJvZHk+PHRyPjx0ZD48L3RkPjx0ZD48L3Rk
PjwvdHI+PC90Ym9keT48L3RhYmxlPg0KPHRhYmxlIHdpZHRoPSI2MDAiIGJvcmRlcj0iMCIgYWxp
Z249ImNlbnRlciI+DQo8dGJvZHk+DQo8dHI+DQo8dGg+PHAgc3R5bGU9InRleHQtYWxpZ246IGNl
bnRlciI+QnJvb2tseW48L3A+DQogPHA+PGEgaHJlZj0iaHR0cDovL2JldGhhbnkuc3UiPjxpbWcg
c3JjPSJodHRwczovL3RoZWNoaXZlLmZpbGVzLndvcmRwcmVzcy5jb20vMjAxOS8wOC9kMzc0Zjcx
NDI0Nzc0MjEwNzdkOWQzZTg4ZmI1OTMxMS5qcGc/cXVhbGl0eT0xMDAmc3RyaXA9aW5mbyZ3PTY0
MSZ6b29tPTIiIHdpZHRoPSIyODAiIGFsdD0ib3BlbiBwcm9maWxlIi8+PC9hPjwvcD48L3RoPg0K
PHRoPjxwIHN0eWxlPSJ0ZXh0LWFsaWduOiBjZW50ZXIiPkVtbWE8L3A+DQogPHA+PGEgaHJlZj0i
aHR0cDovL2JldGhhbnkuc3UiPjxpbWcgc3JjPSJodHRwczovL3RoZWNoaXZlLmZpbGVzLndvcmRw
cmVzcy5jb20vMjAxOS8wOC9kNTk4ZjdlYTYxYWZjYTNjYjg2MjVkN2NmYTE5NzRiNC5qcGc/cXVh
bGl0eT0xMDAmc3RyaXA9aW5mbyZ3PTY0MSZ6b29tPTIiIHdpZHRoPSIyODAiIGFsdD0ib3BlbiBw
cm9maWxlIi8+PC9hPjwvcD48L3RoPg0KPC90cj4NCjx0cj4NCjx0ZD48dGFibGUgd2lkdGg9IjUw
JSIgYm9yZGVyPSIwIj48dGJvZHk+PHRyPjx0ZD48L3RkPjx0ZD48L3RkPjwvdHI+PC90Ym9keT48
L3RhYmxlPjwvdGQ+DQo8dGQ+PHVsPjxwPjwvcD48L3VsPjwvdGQ+DQo8L3RyPg0KPHRyPg0KPHRo
PjxwIHN0eWxlPSJ0ZXh0LWFsaWduOiBjZW50ZXIiPkVtbWE8L3A+DQogPHA+PGEgaHJlZj0iaHR0
cDovL2JldGhhbnkuc3UiPjxpbWcgc3JjPSJodHRwczovL3RoZWNoaXZlLmZpbGVzLndvcmRwcmVz
cy5jb20vMjAxOS8wOS85YzU1ZjA1MmMzZDZhODgyZGYxMTFhZDZhZmFjOWMwNF93aWR0aC02MDAu
anBlZz9xdWFsaXR5PTEwMCZzdHJpcD1pbmZvJnc9NjQxJnpvb209MiIgd2lkdGg9IjI4MCIgYWx0
PSJvcGVuIHByb2ZpbGUiLz48L2E+PC9wPjwvdGg+DQo8dGg+PHAgc3R5bGU9InRleHQtYWxpZ246
IGNlbnRlciI+QXZhPC9wPg0KIDxwPjxhIGhyZWY9Imh0dHA6Ly9iZXRoYW55LnN1Ij48aW1nIHNy
Yz0iaHR0cHM6Ly90aGVjaGl2ZS5maWxlcy53b3JkcHJlc3MuY29tLzIwMTkvMDkvMzdlMDM1ZGZj
YTM2NjkyZTk3ZTA4OWFjN2ZiNWVjN2QuanBnP3F1YWxpdHk9MTAwJnN0cmlwPWluZm8mdz02NDEm
em9vbT0yIiB3aWR0aD0iMjgwIiBhbHQ9Im9wZW4gcHJvZmlsZSIvPjwvYT48L3A+PC90aD4NCjwv
dHI+DQo8L3Rib2R5Pg0KPC90YWJsZT4NCjxuYXY+PC9uYXY+DQo8dGFibGUgc3R5bGU9Im1heC13
aWR0aDo2MDBweDsgIiBjbGFzcz0ibWNuVGV4dENvbnRlbnRDb250YWluZXIiIHdpZHRoPSIxMDAl
IiBjZWxsc3BhY2luZz0iMCIgY2VsbHBhZGRpbmc9IjAiIGJvcmRlcj0iMCIgYWxpZ249ImNlbnRl
ciI+DQo8dGJvZHk+PHRyPg0KPHRkIGNsYXNzPSJtY25UZXh0Q29udGVudCIgc3R5bGU9InBhZGRp
bmctdG9wOjA7IHBhZGRpbmctcmlnaHQ6MThweDsgcGFkZGluZy1ib3R0b206OXB4OyBwYWRkaW5n
LWxlZnQ6MThweDsiIHZhbGlnbj0idG9wIj4NCiAgIDxhIGhyZWY9Imh0dHA6Ly9iZXRoYW55LnN1
L3Vuc3ViL3Vuc3ViLnBocCI+PHRhYmxlIHdpZHRoPSIwOCUiIGJvcmRlcj0iMCI+PHRib2R5Pjx0
cj48dGQ+PC90ZD48dGQ+PC90ZD48L3RyPjwvdGJvZHk+PC90YWJsZT51bnN1YnNjcmliZSBmcm9t
IHRoaXMgbGlzdDwvYT4uPGJyPg0KPC90ZD4NCjwvdHI+DQo8L3Rib2R5PjwvdGFibGU+DQo8L2Jv
ZHk+DQo8L2h0bWw+DQo=`

func TestHashBytes(t *testing.T) {
	var h HashKey
	h.Pack([]byte{222, 23, 3, 128, 1, 23, 3, 128, 1, 23, 3, 255, 1, 23, 3, 128})
	if h.String() != "3hcDgAEXA4ABFwP/ARcDgA" {
		t.Error("expecting 3hcDgAEXA4ABFwP/ARcDgA got", h.String())
	}
}

func TestTransformer(t *testing.T) {
	store, chunksaver, mimeanalyzer, stream := initTestStream(true)
	buf := make([]byte, 64)
	var result bytes.Buffer
	if _, err := io.CopyBuffer(stream, bytes.NewBuffer([]byte(email3)), buf); err != nil {
		t.Error(err)
	} else {
		_ = mimeanalyzer.Close()
		_ = chunksaver.Close()

		email, err := store.GetEmail(1)
		if err != nil {
			t.Error("email not found")
			return
		}

		// this should read all parts
		r, err := NewChunkedReader(store, email, 0)
		buf2 := make([]byte, 64)
		if w, err := io.CopyBuffer(&result, r, buf2); err != nil {
			t.Error(err)
		} else if w != email.size {
			t.Error("email.size != number of bytes copied from reader", w, email.size)
		}

		if !strings.Contains(result.String(), "</html>") {
			t.Error("Looks like it didn;t read the entire email, was expecting </html>")
		}
		result.Reset()
	}
}

func TestChunkSaverReader(t *testing.T) {
	store, chunksaver, mimeanalyzer, stream := initTestStream(false)
	buf := make([]byte, 64)
	var result bytes.Buffer
	if _, err := io.CopyBuffer(stream, bytes.NewBuffer([]byte(email3)), buf); err != nil {
		t.Error(err)
	} else {
		_ = mimeanalyzer.Close()
		_ = chunksaver.Close()

		email, err := store.GetEmail(1)
		if err != nil {
			t.Error("email not found")
			return
		}

		// this should read all parts
		r, err := NewChunkedReader(store, email, 0)
		buf2 := make([]byte, 64)
		if w, err := io.CopyBuffer(&result, r, buf2); err != nil {
			t.Error(err)
		} else if w != email.size {
			t.Error("email.size != number of bytes copied from reader", w, email.size)
		}

		if !strings.Contains(result.String(), "k+DQo8L2h0bWw+DQo") {
			t.Error("Looks like it didn;t read the entire email, was expecting k+DQo8L2h0bWw+DQo")
		}
		result.Reset()

		// Test the decoder, hit the decoderStateMatchNL state
		r, err = NewChunkedReader(store, email, 0)
		if err != nil {
			t.Error(err)
		}
		part := email.partsInfo.Parts[0]

		encoding := transfer.QuotedPrintable
		if strings.Contains(part.TransferEncoding, "base") {
			encoding = transfer.Base64
		}
		dr, err := transfer.NewDecoder(r, encoding, part.Charset)
		_ = dr
		if err != nil {
			t.Error(err)
			t.FailNow()
		}

		buf3 := make([]byte, 1253) // 1253 intentionally causes the decoderStateMatchNL state to hit
		_, err = io.CopyBuffer(&result, dr, buf3)
		if err != nil {
			t.Error()
		}
		if !strings.Contains(result.String(), "</html") {
			t.Error("looks like it didn't decode, expecting </html>")
		}
		result.Reset()

		// test the decoder, hit the decoderStateFindHeaderEnd state
		r, err = NewChunkedReader(store, email, 0)
		if err != nil {
			t.Error(err)
		}
		part = email.partsInfo.Parts[0]
		encoding = transfer.QuotedPrintable
		if strings.Contains(part.TransferEncoding, "base") {
			encoding = transfer.Base64
		}
		dr, err = transfer.NewDecoder(r, encoding, part.Charset)
		_ = dr
		if err != nil {
			t.Error(err)
			t.FailNow()
		}

		buf4 := make([]byte, 64) // state decoderStateFindHeaderEnd will hit
		_, err = io.CopyBuffer(&result, dr, buf4)
		if err != nil {
			t.Error()
		}
		if !strings.Contains(result.String(), "</html") {
			t.Error("looks like it didn't decode, expecting </html>")
		}

	}

}

func TestChunkSaverWrite(t *testing.T) {

	store, chunksaver, mimeanalyzer, stream := initTestStream(true)
	var out bytes.Buffer
	buf := make([]byte, 128)
	if written, err := io.CopyBuffer(stream, bytes.NewBuffer([]byte(email)), buf); err != nil {
		t.Error(err)
	} else {
		_ = mimeanalyzer.Close()
		_ = chunksaver.Close()
		fmt.Println("written:", written)
		total := 0
		for _, chunk := range store.chunks {
			total += len(chunk.data)
		}
		fmt.Println("compressed", total, "saved:", written-int64(total))
		email, err := store.GetEmail(1)
		if err != nil {
			t.Error("email not found")
			return
		}

		// this should read all parts
		r, err := NewChunkedReader(store, email, 0)
		if w, err := io.Copy(&out, r); err != nil {
			t.Error(err)
		} else if w != email.size {
			t.Error("email.size != number of bytes copied from reader", w, email.size)
		} else if !strings.Contains(out.String(), "GIF89") {
			t.Error("The email didn't decode properly, expecting GIF89")
		}
		out.Reset()

		// test the seek feature
		r, err = NewChunkedReader(store, email, 0)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		// we start from 1 because if the start from 0, all the parts will be read
		for i := 1; i < len(email.partsInfo.Parts); i++ {
			fmt.Println("seeking to", i)
			err = r.SeekPart(i)
			if err != nil {
				t.Error(err)
			}
			w, err := io.Copy(&out, r)
			if err != nil {
				t.Error(err)
			}
			if w != int64(email.partsInfo.Parts[i-1].Size) {
				t.Error(i, "incorrect size, expecting", email.partsInfo.Parts[i-1].Size, "but read:", w)
			}
			out.Reset()
		}

		r, err = NewChunkedReader(store, email, 0)
		if err != nil {
			t.Error(err)
		}
		part := email.partsInfo.Parts[0]
		encoding := transfer.QuotedPrintable
		if strings.Contains(part.TransferEncoding, "base") {
			encoding = transfer.Base64
		}
		dr, err := transfer.NewDecoder(r, encoding, part.Charset)
		_ = dr
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		//var decoded bytes.Buffer
		//io.Copy(&decoded, dr)
		io.Copy(os.Stdout, dr)

	}
}

func initTestStream(transform bool) (*StoreMemory, *backends.StreamDecorator, *backends.StreamDecorator, backends.StreamProcessor) {
	// place the parse result in an envelope
	e := mail.NewEnvelope("127.0.0.1", 1, 234)
	to, _ := mail.NewAddress("test@test.com")
	e.RcptTo = append(e.RcptTo, *to)
	from, _ := mail.NewAddress("test@test.com")
	e.MailFrom = *from
	store := new(StoreMemory)
	chunkBuffer := NewChunkedBytesBufferMime()
	//chunkBuffer.setDatabase(store)
	// instantiate the chunk saver
	chunksaver := backends.Streamers["chunksaver"]()
	mimeanalyzer := backends.Streamers["mimeanalyzer"]()
	transformer := backends.Streamers["transformer"]()
	//debug := backends.Streamers["debug"]()
	// add the default processor as the underlying processor for chunksaver
	// and chain it with mimeanalyzer.
	// Call order: mimeanalyzer -> chunksaver -> default (terminator)
	// This will also set our Open, Close and Initialize functions
	// we also inject a Storage and a ChunkingBufferMime
	var stream backends.StreamProcessor
	if transform {
		stream = mimeanalyzer.Decorate(
			transformer.Decorate(
				//debug.Decorate(
				chunksaver.Decorate(
					backends.DefaultStreamProcessor{}, store, chunkBuffer)))
	} else {
		stream = mimeanalyzer.Decorate(
			//debug.Decorate(
			chunksaver.Decorate(
				backends.DefaultStreamProcessor{}, store, chunkBuffer)) //)
	}

	// configure the buffer cap
	bc := backends.BackendConfig{
		backends.ConfigStreamProcessors: {
			"chunksaver": {
				"chunk_size":     8000,
				"storage_engine": "memory",
				"compress_level": 9,
			},
		},
	}

	//_ = backends.Svc.Initialize(bc)
	_ = chunksaver.Configure(bc[backends.ConfigStreamProcessors]["chunksaver"])
	_ = mimeanalyzer.Configure(backends.ConfigGroup{})
	// give it the envelope with the parse results
	_ = chunksaver.Open(e)
	_ = mimeanalyzer.Open(e)
	if transform {
		_ = transformer.Open(e)
	}

	return store, chunksaver, mimeanalyzer, stream
}
