package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(Command interface{}) (Index, CurrentTerm, isleader)
//   start agreement on a new Log entry
// rf.GetState() (CurrentTerm, leaderStatus)
//   ask a Raft for its current CurrentTerm, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the Log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"../labgob"
	"../labrpc"
	"bytes"
	"log"
	"math/rand"
	"sync"
	"time"
)
import "sync/atomic"

// import "bytes"
// import "../labgob"

//
// as each Raft peer becomes aware that successive Log Entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed Log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

// todo value
var electionTimeout = time.Duration(time.Millisecond * 200)

func (rf *Raft) randomElectionTimeout() time.Duration {
	//seed := int64((rf.me * 13) * int(rf.status.CurrentTerm*17))
	//rand.Seed(seed)
	//log.Printf("debug %d rand seed is %d", rf.me, seed)

	res := time.Duration(int64(float64(electionTimeout) + (rf.rand.Float64() * float64(time.Duration(time.Millisecond*400)))))
	//log.Printf("debug %d timeout is %d", rf.me, res)
	return res

}

// todo value
var HeatBeatTimeout = time.Duration(time.Millisecond * 120)

const (
	leader    = "leader"
	follower  = "follower"
	candidate = "candidate"
)

const appendConflictDecreaseNumber = 200

//
// A Go object implementing a single Raft peer.
//

type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	// all data need persist
	applyMsgChan        chan ApplyMsg
	ApplyMsgUnblockChan chan ApplyMsg
	status              Status
	peerStatusMap       map[int]PeerStatus
	appendEntryRequest  chan AppendEntryArgs
	appendEntryReply    chan AppendEntryReply
	voteReplyChan       chan RequestVoteReply
	voteRequestChan     chan RequestVoteArgs
	startRequestChan    chan interface{}
	// todo
	startReplyChan chan StartReply

	followersInfo map[int]FollowerInfo

	// just for lab test
	leaderStatus   bool
	committeeIndex Index
	lastApply      Index

	rand *rand.Rand
}

func (rf *Raft) serverRoutine() {

}

// return CurrentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	// Your code here (2A).
	term = int(rf.getCurrentTerm())
	isleader = rf.isLeader()

	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.status)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	var status Status
	if d.Decode(&status) != nil {
		log.Fatalf("%d read persiste error ", rf.me)
		//    d.Decode(&yyy) != nil {
		//   error...
	} else {
		rf.status = status
	}
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	Term          Term
	LastSlotIndex Index
	LastSlotTerm  Term
	Id            int
	// Your data here (2A, 2B).
}

const (
	voteReplySuccess                       = "voteReplySuccess"
	voteReplyApplyAlreadyVote              = "voteReplyApplyAlreadyVote"
	voteReplyStaleTerm                     = "voteReplyStaleTerm"
	voteReplyLatestLogEntryIsNotUpdateToMe = "voteReplyLatestLogEntryIsNotUpdateToMe"
)

//
// example RequestVote RPC Reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	Result      string
	CurrentTerm Term
	// Your data here (2A).
}

type AppendEntryArgs struct {
	PreviousEntryIndex Index
	PreviousEntryTerm  Term
	Entries            []Entry
	CurrentTerm        Term
	LeaderCommittee    Index
}

const (
	appendEntryAccept    = 1
	appendEntryNotMatch  = 2
	appendEntryStaleTerm = 3
)

type AppendEntryResult int

type AppendEntryReply struct {
	Id        int
	Result    AppendEntryResult
	Term      Term
	LastIndex Index
}

func (rf *Raft) sendAppendEntry(server int, args *AppendEntryArgs, reply *AppendEntryReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntry", args, reply)
	return ok
}

func (rf *Raft) AppendEntry(args *AppendEntryArgs, reply *AppendEntryReply) {
	rf.appendEntryRequest <- *args
	*reply = <-rf.appendEntryReply
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in Args.
// fills in *Reply with RPC Reply, so caller should
// pass &Reply.
// the types of the Args and Reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a Reply. If a Reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost Reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the Reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	//log.Printf("%d receive vote request from %d", rf.me, args.Id)
	rf.voteRequestChan <- *args
	*reply = <-rf.voteReplyChan
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next Command to be appended to Raft's Log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// Command will ever be committed to the Raft Log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the Command will appear at
// if it's ever committed. the second return value is the current
// CurrentTerm. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	if !rf.isLeader() {
		isLeader = false
		return index, term, isLeader
	}

	// Your code here (2B).
	rf.startRequestChan <- command
	reply := <-rf.startReplyChan
	//log.Printf("%d receive start command, return term %d ,Index %d", rf.me, reply.Term, reply.Index)

	return reply.Index, reply.Term, isLeader
}

//
// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
//
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.status = NewStatus()
	rf.appendEntryRequest = make(chan AppendEntryArgs)
	rf.appendEntryReply = make(chan AppendEntryReply)
	rf.voteReplyChan = make(chan RequestVoteReply)
	rf.voteRequestChan = make(chan RequestVoteArgs)
	rf.peerStatusMap = make(map[int]PeerStatus)
	rf.startRequestChan = make(chan interface{})
	rf.startReplyChan = make(chan StartReply)
	rf.leaderStatus = false
	rf.committeeIndex = 0
	rf.lastApply = 0
	rf.applyMsgChan = applyCh
	rf.ApplyMsgUnblockChan = make(chan ApplyMsg, 1000)
	seed := time.Now().UnixNano()
	rf.rand = rand.New(rand.NewSource(seed + int64(13*rf.me)))
	rf.lastApply = 0

	// Your initialization code here (2A, 2B, 2C).

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	go rf.applyMsgRoutine()
	go rf.mainRoutine()

	return rf
}
