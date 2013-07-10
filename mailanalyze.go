package mailanalyze

import (
	"code.google.com/p/go.net/html"
 	"github.com/taruti/langdetect"
 	"github.com/taruti/langword"
	"github.com/sloonz/go-qprintable"
	"bitbucket.org/taruti/snowball"
	"bufio"
	"bytes"
	"encoding/base64"
//	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"regexp"
	"strings"
)

type MailInfo struct {
	Subject string
	MailingList string
	Precedence Precedence
	Thread []string
	Senders []string
	Destinations []string
	MessageID string
	ContentType string
	Language langdetect.Language
	BodyWords map[string]int
	HeaderWords map[HeaderWord]int
}

type HeaderWord struct {
	Header string
	Word string
}

type Precedence int

const (
	Spam = Precedence(0)
	Junk = Precedence(1)
	List = Precedence(5)
	Personal  = Precedence(10)
)


func Analyze(inp io.Reader) (*MailInfo, error) {
	res := &MailInfo{}
	mlnoat := true
	msg,e := mail.ReadMessage(inp)
	var delivto string
	var contentType, contentTransferEncoding string
	if e!=nil {
		return nil,e
	}
	res.Precedence = Personal
	for k,arr := range msg.Header {
		if k == "" || k[0] == 'X' {
			continue
		}
		switch k {
		case `Errors-To` , `Old-Received-Spf` , `Received-Spf` , `Dkim-Signature` , `Received`,
			`Return-Path` , `Domainkey-Signature` , `User-Agent`, `Importance`, `Content-Disposition`,
			`Cancel-Lock`:
			// skip...
		case  `In-Reply-To`,  `References`:
			res.Thread = append(res.Thread, arr...) //fixme need moar splitting for references
		case  `Mime-Version`:
		case  `Content-Type`:
			contentType = arr[0]
		case  `Content-Transfer-Encoding`:
			contentTransferEncoding = arr[0]
		case  `Date`:
			// FIXME
		case  `Subject`:
			res.Subject = decodeEncodedWord(arr[0])
		case  `Message-Id`:
			res.MessageID = arr[0]
		case  `Precedence`:
			switch arr[0] {
			case "junk", "bulk": res.Precedence = Junk
			case "list":         res.Precedence = List
			}
		case  `Delivered-To`:
			delivto = arr[0]
		case  `Cc`, `Bcc`, `To`:
			res.Destinations = append(res.Destinations, arr...) //fixme need moar splitting
		case `Sender`, `From`, `Reply-To`, `Mail-Followup-To`:
			res.Senders = append(res.Senders, arr[0])
		case  `List-Unsubscribe`,  `List-Help`,  `List-Subscribe`,  `List-Archive`,  `List-Owner`, `List-Post`:
			res.Precedence = precedenceMin(res.Precedence, List)
		case `List-Id`:
			res.Precedence = precedenceMin(res.Precedence, List)
			i := strings.Index(arr[0], "<")
			j := strings.Index(arr[0], ">")
			if i >= 0 && j > i {
				ml := arr[0][i+1:j]
				if newat := strings.Index(ml, "@")>0; mlnoat {
					mlnoat = !newat
					res.MailingList = ml
				}
			}
		case `Mailing-List`:
			res.Precedence = precedenceMin(res.Precedence, List)
			for _,v := range arr {
				if strings.HasPrefix(v,"list ") {
					if idx := strings.Index(v, ";"); idx>=0 {
						ml := v[5:idx-1]
						if newat := strings.Index(ml, "@")>0; mlnoat {
							mlnoat = !newat
							res.MailingList = ml
						}
					}
				}
			}
		default:
//			fmt.Println("UNHANDLED: ",k,arr)
			
		}
	}
	if len(res.Destinations)==0 {
		res.Destinations = []string{delivto}
	}
	res.BodyWords = map[string]int{}
	res.HeaderWords = map[HeaderWord]int{}

	res.ContentType,res.Language = parseBody(res.BodyWords, contentType, contentTransferEncoding, msg.Body)

	handleHeaderWords(res.HeaderWords, `title`, []string{res.Subject}, res.Language)
	handleHeaderWords(res.HeaderWords, `sender`, res.Senders, langdetect.Unknown)
	handleHeaderWords(res.HeaderWords, `dest`, res.Destinations, langdetect.Unknown)

	res.HeaderWords[HeaderWord{`lang`,res.Language.String()}] = 1
	res.HeaderWords[HeaderWord{`list`,res.MailingList}] = 1

	return res,nil
}

//var _ = fmt.Println

func precedenceMin(a, b Precedence) Precedence {
	if a < b {
		return a
	}
	return b
}


var ewRe = regexp.MustCompile(`=\?[\w-]+?\?(Q|B)\?.+?\?=`)
func ewReFun(s string) string {
	inner := s[2:len(s)-2]
	i := strings.Index(inner, "?")
	enc := inner[0:i]
	char:= inner[i+1]
	inner = inner[i+3:]
	switch char {
	case 'B':
		bs,e := base64.StdEncoding.DecodeString(inner)
		if e == nil {
			rd,_,e := langdetect.CharsetToUtf8(enc,bytes.NewReader(bs))
			if e==nil {
				return readAllString(rd)
			}
		}
	case 'Q':
		rd,_,e := langdetect.CharsetToUtf8(enc,bytes.NewReader(deqencode(inner)))
		if e==nil {
			return readAllString(rd)
		}
	}
	return s
}

func decodeEncodedWord(s string) string {
	return ewRe.ReplaceAllStringFunc(s, ewReFun)
}

func readAllString(rd io.Reader) string {
	bs := new(bytes.Buffer)
	io.Copy(bs, rd)
	return bs.String()
}

func deqencode(s string) []byte {
	res := make([]byte, 0, len(s))
	for i:=0; i<len(s); i++ {
		switch s[i] {
		case '=':
			if i+2 < len(s) {
				res = append(res, (hex2int(s[i+1])<<4) + hex2int(s[i+2]))
				i += 2
			}
		case '_':
			res = append(res, ' ')
		default:
			res = append(res, s[i])
		}
	}
	return res
}

func hex2int(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return (c - '0')
	case c >= 'A' && c <= 'F':
		return 10 + (c - 'A') 
	case c >= 'a' && c <= 'f':
		return 10 + (c - 'a')
	}
	return 0
}

func decontenttransfer(cte string, rd io.Reader) io.Reader {
	switch cte {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, rd)
	case "quoted-printable":
		return qprintable.NewDecoder(qprintable.BinaryEncoding, rd)
	}
	return rd
}

func dumpHtmlBodyTo(dest *bytes.Buffer, rd io.Reader) {
	tker := html.NewTokenizer(rd)
	seenBody := false
	for tker.Err()==nil {
		tok := tker.Next()
		switch tok {
		case html.StartTagToken:
			nbs,_ := tker.TagName()
			n := string(nbs)
			if n == `body` || n==`div` || n==`p` {
				seenBody = true
			}
		case html.TextToken:
			if seenBody {
				dest.Write(tker.Text())
				if dest.Len()>128*1024 {
					break
				}
			}
		}
	}
}


func parseBody(bodywords map[string]int, contentType string, contentTransferEncoding string, body io.Reader) (mimetype string, language langdetect.Language) {
	var e error
	var m map[string]string
	mimetype,m,_ = mime.ParseMediaType(contentType)
	var enc string
	if m!=nil {
		enc,_ = m[`charset`]
	}
	var rd io.Reader
	var ct string
	if !strings.HasPrefix(mimetype, "multipart/") {
		r0 := decontenttransfer(contentTransferEncoding, body)
		rd,_,e = langdetect.CharsetToUtf8(enc,r0)
		if e!=nil {
			rd = r0
		}
		ct = mimetype
	} else if boundary,ok := m[`boundary`]; ok {
		mr := multipart.NewReader(body, boundary)
		for {
			part,e := mr.NextPart()
			if e!=nil {
				return
			}
			cty,_ := part.Header[`Content-Type`]
			cte,_ := part.Header[`Content-Transfer-Encoding`]
			_, l := parseBody(bodywords, mzs(cty), mzs(cte), part)
			if language == langdetect.Unknown {
				language = l
			}
		}
		return
	}
	bs := new(bytes.Buffer)
	switch ct {
	case "":
	case "text/plain":
		io.CopyN(bs, rd, 128*1024)
	case "text/html":
		dumpHtmlBodyTo(bs, rd)
	default:
//		fmt.Println("CTTT ", ct)
	}
	if bs.Len()>0 {
		language = langdetect.DetectLanguage(bs.Bytes(),enc)
		scan := bufio.NewScanner(bs)
		scan.Split(langword.ScanLatinWords)
		ser,sere := snowball.New(language.String())
		for scan.Scan() {
			w := scan.Text()
			if len(w)>=3 && len(w)<32 && !langdetect.IsCommonWord(language, w) {
				if sere == nil {
					bodywords[ser.Stem(w)]++
				} else {
					bodywords[w]++
				}
			}
		}
	}
	return
}

func mzs(ss []string) string {
	if len(ss)==0 {
		return ""
	}
	return ss[0]
}

var emailRe = regexp.MustCompile(`\b[\w\.\%\+\-]+@[\w\.]+\.[A-Za-z]{2,5}\b`)
func handleHeaderWords(m map[HeaderWord]int, name string, values []string, language langdetect.Language) {
	value := strings.Join(values, " ")
	addrs := emailRe.FindAllString(value, -1)
	for _,addr := range addrs {
		m[HeaderWord{name,addr}]++
	}
	scan := bufio.NewScanner(strings.NewReader(value))
	scan.Split(langword.ScanLatinWords)
	ser,sere := snowball.New(language.String())
	for scan.Scan() {
		w := scan.Text()
		if len(w)>=3 && len(w)<32 && !langdetect.IsCommonWord(language, w) {
			if sere == nil {
				m[HeaderWord{name,ser.Stem(w)}]++
			} else {
				m[HeaderWord{name,w}]++
			}
		}
	}
}
