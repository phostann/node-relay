package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type NodeMsgType string

const (
	Ping     NodeMsgType = "ping"
	InitNode NodeMsgType = "init_node"
)

// WebSocketManager 管理所有WebSocket连接和相关操作
// 将全局变量封装到结构体中，便于管理和测试
type WebSocketManager struct {
	// 从节点ID到WebSocket连接的映射
	nodeConnections map[string][]*websocket.Conn
	// 保护nodeConnections的互斥锁
	connMutex sync.RWMutex
	// WebSocket升级器
	upgrader websocket.Upgrader
}

// NewWebSocketManager 创建并初始化一个新的WebSocket管理器
func NewWebSocketManager() *WebSocketManager {
	return &WebSocketManager{
		nodeConnections: make(map[string][]*websocket.Conn),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			// 允许所有来源的跨域请求
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (m *WebSocketManager) GetNodeConnById(uid string) []*websocket.Conn {
	m.connMutex.RLock()
	defer m.connMutex.RUnlock()

	return m.nodeConnections[uid]
}

// 全局WebSocket管理器实例
var wsManager = NewWebSocketManager()

// SetupSocketRoutes 设置WebSocket相关路由
func SetupSocketRoutes(router *gin.RouterGroup) {
	router.GET("/node/:uid", HandleNodeSocket)
}

// HandleNodeSocket 处理节点WebSocket连接请求
// 接收来自客户端的WebSocket连接，并将其关联到特定节点ID
func HandleNodeSocket(c *gin.Context) {
	// 获取节点ID参数
	uid := c.Param("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "缺少节点ID参数",
		})
		return
	}

	// 验证节点ID的有效性
	if !checkUid(uid) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的节点ID",
		})
		return
	}

	// 升级HTTP连接为WebSocket连接
	ws, err := wsManager.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "升级为WebSocket失败: " + err.Error(),
		})
		return
	}

	// 将WebSocket连接添加到对应节点的连接列表
	wsManager.AddConnection(uid, ws)

	// 在单独的goroutine中处理连接
	go wsManager.handleConnection(ws, uid)
}

// AddConnection 添加WebSocket连接到指定节点
func (m *WebSocketManager) AddConnection(uid string, ws *websocket.Conn) {
	m.connMutex.Lock()
	defer m.connMutex.Unlock()

	m.nodeConnections[uid] = append(m.nodeConnections[uid], ws)
}

// RemoveConnection 从指定节点移除WebSocket连接
func (m *WebSocketManager) RemoveConnection(uid string, ws *websocket.Conn) {
	m.connMutex.Lock()
	defer m.connMutex.Unlock()

	connections, exists := m.nodeConnections[uid]
	if !exists {
		return
	}

	// 查找并移除指定连接
	for i, conn := range connections {
		if conn == ws {
			// 使用切片操作移除元素
			m.nodeConnections[uid] = append(connections[:i], connections[i+1:]...)
			break
		}
	}

	// 如果节点没有剩余连接，则删除该节点的映射
	if len(m.nodeConnections[uid]) == 0 {
		delete(m.nodeConnections, uid)
	}
}

// SendMessageToNode 向指定节点的所有连接发送消息
// 这是一个公共函数，可以被其他包调用来向特定节点发送消息
func SendMessageToNode(uid string, message []byte) {
	wsManager.SendMessage(uid, message)
}

// SendMessage 向指定节点的所有连接发送消息
func (m *WebSocketManager) SendMessage(uid string, message []byte) {
	m.connMutex.RLock()
	defer m.connMutex.RUnlock()

	connections, exists := m.nodeConnections[uid]
	if !exists {
		return
	}

	for _, conn := range connections {
		// 使用非阻塞方式发送消息，避免一个连接卡住影响其他连接
		go func(c *websocket.Conn, msg []byte) {
			if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
				// 发送失败时记录错误，但不中断其他连接的发送
				fmt.Printf("向节点 %s 发送消息失败: %v\n", uid, err)
			}
		}(conn, message)
	}
}

// checkUid 验证节点ID的有效性
func checkUid(uid string) bool {
	// TODO: 实现更严格的节点ID验证逻辑
	// 例如：检查格式、查询数据库验证存在性等
	fmt.Println("正在验证节点ID:", uid)
	return true
}

// handleConnection 处理WebSocket连接的生命周期
func (m *WebSocketManager) handleConnection(ws *websocket.Conn, uid string) {
	// 确保连接关闭和资源清理
	defer func() {
		ws.Close()
		m.RemoveConnection(uid, ws)
		fmt.Printf("节点 %s 的一个连接已关闭\n", uid)
	}()

	// 设置关闭处理器
	ws.SetCloseHandler(func(code int, text string) error {
		fmt.Printf("节点 %s 的连接正常关闭，代码: %d, 原因: %s\n", uid, code, text)
		// 关闭连接时移除该连接
		m.RemoveConnection(uid, ws)
		return nil
	})

	// 设置Ping处理器，保持连接活跃
	ws.SetPingHandler(func(appData string) error {
		// 响应Ping消息，发送Pong
		err := ws.WriteControl(websocket.PongMessage, []byte{}, time.Now().Add(10*time.Second))
		if err != nil {
			fmt.Printf("发送Pong消息失败: %v\n", err)
		}
		return err
	})

	// 持续读取消息，直到连接关闭
	for {
		msgType, msg, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
				websocket.CloseNoStatusReceived) {
				fmt.Printf("节点 %s 连接异常关闭: %v\n", uid, err)
			}
			break
		}

		// 处理文本消息
		if msgType == websocket.TextMessage {
			m.handleTextMsg(ws, msg, uid)
		}
	}
}

// TextMsg 定义WebSocket文本消息的基本结构
type TextMsg struct {
	Type NodeMsgType  `json:"type"` // 消息类型
	Data *interface{} `json:"data"` // 消息数据
}

// handleTextMsg 处理接收到的文本消息
func (m *WebSocketManager) handleTextMsg(ws *websocket.Conn, msg []byte, uid string) {
	var textMsg TextMsg
	err := json.Unmarshal(msg, &textMsg)
	if err != nil {
		fmt.Printf("解析来自节点 %s 的消息失败: %v\n", uid, err)
		return
	}

	// 根据消息类型分发处理
	switch textMsg.Type {
	case Ping:
		m.handlePing(ws)
	case InitNode:
		m.handleInitNode(textMsg.Data)
	// 可以在此添加更多消息类型的处理分支
	default:
		fmt.Printf("收到未知类型的消息: %s\n", textMsg.Type)
	}
}

// handlePing 处理ping消息，回复pong消息
func (m *WebSocketManager) handlePing(ws *websocket.Conn) {
	resp := map[string]string{
		"type":      "pong",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	bytes, err := json.Marshal(resp)
	if err != nil {
		fmt.Println("序列化pong消息失败:", err)
		return
	}

	if err := ws.WriteMessage(websocket.TextMessage, bytes); err != nil {
		fmt.Println("发送pong消息失败:", err)
	}
}

// handleInitNode 处理初始化节点消息
func (m *WebSocketManager) handleInitNode(data *interface{}) {
	// 将 data 强转为字符串
	uid, ok := (*data).(string)
	if !ok {
		fmt.Println("data 不是字符串")
		return
	}
	wsManager.connMutex.Lock()
	nodeConn := wsManager.nodeConnections[uid]
	if nodeConn == nil {
		fmt.Println("节点不存在")
		return
	}
	wsManager.connMutex.Unlock()
	wsManager.SendMessage(uid, []byte("init_node_success"))
	wsManager.SendMessage("111111", []byte("init_node_success"))

}
