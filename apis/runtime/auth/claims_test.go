package auth

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestUserClaimsUnmarshalBothFormats 钉住 claims 的两种编码都能解析。
//
// 为什么必须两种都认：token 的签发方与校验方不一定同栈（业务由 Node 签发、由 Go 校验），
// 只认紧凑键时标准格式的 token 会解析成全零 claims —— 表现为"任何合法 token 都验不过"，
// 而且没有任何编译期提示。
func TestUserClaimsUnmarshalBothFormats(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    UserClaims
	}{
		{
			name:    "yg-go 历史紧凑键",
			payload: `{"c":42,"t":1700000000,"e":1700003600,"i":"snapnow","a":"business-api","l":1}`,
			want: UserClaims{
				Uin:       42,
				IssuedAt:  1700000000,
				ExpiresAt: 1700003600,
				Issuer:    "snapnow",
				Audience:  "business-api",
				LoginWay:  LoginWayEmail,
			},
		},
		{
			name:    "标准 JWT 声明",
			payload: `{"uin":42,"iat":1700000000,"exp":1700003600,"iss":"snapnow","aud":"business-api","login_way":2,"sid":"sess-1","session_version":7}`,
			want: UserClaims{
				Uin:            42,
				IssuedAt:       1700000000,
				ExpiresAt:      1700003600,
				Issuer:         "snapnow",
				Audience:       "business-api",
				LoginWay:       LoginWayPhone,
				SessionID:      "sess-1",
				SessionVersion: 7,
			},
		},
		{
			// 同一份 payload 里两种键共存：标准键非零才覆盖，避免把紧凑键的值踩成零。
			name:    "两种键共存时标准键优先",
			payload: `{"c":1,"uin":42,"i":"compact","iss":"standard","l":1,"login_way":2,"session_version":3}`,
			want: UserClaims{
				Uin:            42,
				Issuer:         "standard",
				LoginWay:       LoginWayPhone,
				SessionVersion: 3,
			},
		},
		{
			// 标准键缺省（零值）时不能覆盖紧凑键。
			name:    "标准键为零值时不覆盖紧凑键",
			payload: `{"c":9,"i":"compact","a":"aud"}`,
			want:    UserClaims{Uin: 9, Issuer: "compact", Audience: "aud"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got UserClaims
			if err := json.Unmarshal([]byte(tc.payload), &got); err != nil {
				t.Fatalf("解析失败：%v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("解析结果不符：\n got = %#v\nwant = %#v", got, tc.want)
			}
		})
	}
}
