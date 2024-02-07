# Chzzk-Chat
네이버 스트리밍 플랫폼인 치지직의 채팅을 가져올 수 있는 Go 라이브러리입니다. 현재 계속 작성중이며, 치지직 역시 베타서비스이므로 언제든지 라이브러리가 동작하지 않을 수 있습니다.

`chzzkcord-server` 와 연계하여 사용하기 위해 만들었기 때문에 응답받는 JSON 데이터를 구조체로 변환하는 과정에서 일부 데이터는 제외하였으며 누락된 데이터도 있을 수 있습니다.

### 사용 방법
```Bash
$ go get -u github.com/chzzkcord/chzzk-chat
```

```Go
package main

import (
    "fmt" 
    
    chzzkchat "github.com/chzzkcord/chzzk-chat"
)

func main() {
    token := chzzkchat.Token{
        Access: {CHZZK_ACCESS_TOKEN},
        UserID: {CHZZK_USER_ID_HASH},
        ChannelID: {CHZZK_CHAT_CHANNEL_ID},
    }

    client := chzzkchat.NewClient(token)
    client.OnChatMessage(func(messages []chzzkchat.ChatMessage) {
        for _, message := range messages {
            fmt.Printf("%s(%s) -> %s\n", message.User.Nickname, message.User.Hash, message.User.Message)
        }
    })
    err := c.Connect()
    if err != nil {
        // 재접속 및 에러처리
    }
}
```

### 콜백
```go
Client.OnChatMessage(messages []ChatMessage)
```
- [93101] 채팅 메시지를 반환합니다.

