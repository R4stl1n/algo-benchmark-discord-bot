package managers

import (
	"github.com/bwmarrin/discordgo"
	"github.com/olekukonko/tablewriter"
	"github.com/r4stl1n/algo-benchmark-discord-bot/pkg/dto"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"sort"
	"strings"
	"time"
)

type ServiceManager struct {
	Config         *dto.ConfigStruct
	DiscordClient  *discordgo.Session
	DatabaseClient *DatabaseManager
}

func CreateServiceManager(config *dto.ConfigStruct, databaseClient *DatabaseManager) *ServiceManager {

	return &ServiceManager{
		Config:         config,
		DatabaseClient: databaseClient,
	}

}

func (serviceManager *ServiceManager) Initialize() error {

	discordClient, discordClientError := discordgo.New("Bot " + serviceManager.Config.BotToken)

	if discordClientError != nil {
		return discordClientError
	}

	serviceManager.DiscordClient = discordClient

	serviceManager.DiscordClient.AddHandler(serviceManager.messageHandler)

	serviceManager.DiscordClient.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsAllWithoutPrivileged)

	discordWebsocketError := serviceManager.DiscordClient.Open()

	if discordWebsocketError != nil {
		return discordWebsocketError
	}

	return nil
}

func (serviceManager *ServiceManager) handleRegisterCommand(s *discordgo.Session, m *discordgo.MessageCreate) {

	chanCreate, chanCreateError := s.UserChannelCreate(m.Author.ID)

	if chanCreateError != nil {
		logrus.Error(chanCreateError)
		return
	}

	if serviceManager.DatabaseClient.CheckIfParticipantExist(m.Author.ID) == true {
		s.ChannelMessageSend(chanCreate.ID, "You are already registered")
		return
	}

	participantModel, participantRegisterError := serviceManager.DatabaseClient.CreateParticipant(m.Author.ID, m.Author.Username)

	if participantRegisterError != nil {
		s.ChannelMessageSend(chanCreate.ID, "Something broke tell the owner you can't register")
	}

	s.ChannelMessageSend(chanCreate.ID, "Welcome to the algo-benchmark")
	s.ChannelMessageSend(chanCreate.ID, "Your participant ID: "+participantModel.UUID+"\n"+"Your rest api key: "+participantModel.ApiKey+"\n"+"Rest Api Endpoint: "+serviceManager.Config.RootURL)
}

func (serviceManager *ServiceManager) handleGiveInfoCommand(s *discordgo.Session, m *discordgo.MessageCreate) {

	chanCreate, chanCreateError := s.UserChannelCreate(m.Author.ID)

	if chanCreateError != nil {
		logrus.Error(chanCreateError)
		return
	}

	if serviceManager.DatabaseClient.CheckIfParticipantExist(m.Author.ID) != true {
		s.ChannelMessageSend(chanCreate.ID, "You are not registered")
		return
	}

	participantModel, participantError := serviceManager.DatabaseClient.GetParticipant(m.Author.ID)

	if participantError != nil {
		logrus.Error(participantError)
		s.ChannelMessageSend(chanCreate.ID, "Something broke tell the owner you can't get your id")
	}

	s.ChannelMessageSend(chanCreate.ID, "Your participant ID: "+participantModel.UUID+"\n"+"Your rest api key: "+participantModel.ApiKey+"\n"+"Rest Api Endpoint: "+serviceManager.Config.RootURL)
}

func (serviceManager *ServiceManager) handleSubmitRoiCommand(s *discordgo.Session, m *discordgo.MessageCreate) {

	chanCreate, chanCreateError := s.UserChannelCreate(m.Author.ID)

	if chanCreateError != nil {
		logrus.Error(chanCreateError)
		return
	}

	if serviceManager.DatabaseClient.CheckIfParticipantExist(m.Author.ID) != true {
		s.ChannelMessageSend(chanCreate.ID, "You are not registered")
		return
	}

	participant, participantError := serviceManager.DatabaseClient.GetParticipant(m.Author.ID)

	if participantError != nil {
		logrus.Error(participantError)
		s.ChannelMessageSend(chanCreate.ID, "Something broke tell the owner you can't get your id")
		return
	}

	if participant.Approved != true {
		s.ChannelMessageSend(chanCreate.ID, "You need to be approved by another approved user before you can submit")
		return
	}

	contentSplit := strings.Split(m.Content, " ")

	if len(contentSplit) != 2 {
		s.ChannelMessageSend(chanCreate.ID, "Command incorrect ex. !registerRoi 123.45")
		return
	}

	submittedValue, submittedValueError := decimal.NewFromString(contentSplit[1])

	if submittedValueError != nil {
		s.ChannelMessageSend(chanCreate.ID, "Submitted value is invalid")
		return
	}

	submittedConv, _ := submittedValue.Round(3).Float64()

	latestSubmission, latestSubmissionError := serviceManager.DatabaseClient.GetCurrentDaySubmissionForParticipant(participant.UUID)

	if latestSubmissionError == nil {

		entryError := serviceManager.DatabaseClient.UpdateLatestEntryForParticipant(participant.UUID, submittedConv)

		if entryError != nil {
			logrus.Error(entryError)
			s.ChannelMessageSend(chanCreate.ID, "Something broke tell the owner you can't submit a roi entry")
			return
		}

		// Entry was made now we need to calculate the dail bm
		serviceManager.updateDailyBmEntry(submittedConv)

		s.ChannelMessageSend(chanCreate.ID, "Update Accepted - Submission ID: "+latestSubmission.UUID)

	} else {
		entryUUID, entryError := serviceManager.DatabaseClient.CreateRoiEntry(participant.UUID, submittedConv)

		if entryError != nil {
			logrus.Error(entryError)
			s.ChannelMessageSend(chanCreate.ID, "Something broke tell the owner you can't submit a roi entry")
			return
		}

		// Entry was made now we need to calculate the dail bm
		serviceManager.updateDailyBmEntry(submittedConv)

		s.ChannelMessageSend(chanCreate.ID, "Submission Accepted - Submission ID: "+entryUUID)
	}
}

func (serviceManager *ServiceManager) handleDailyBmCommand(s *discordgo.Session, m *discordgo.MessageCreate) {

	chanCreate, chanCreateError := s.UserChannelCreate(m.Author.ID)

	if chanCreateError != nil {
		logrus.Error(chanCreateError)
		return
	}

	if serviceManager.DatabaseClient.CheckIfParticipantExist(m.Author.ID) != true {
		s.ChannelMessageSend(chanCreate.ID, "You are not registered")
		return
	}

	dailyBmEntry, dailyBmEntryError := serviceManager.DatabaseClient.GetDailyBmForToday()

	if dailyBmEntryError != nil {
		s.ChannelMessageSend(m.ChannelID, "No submissions for today")
		return
	}

	s.ChannelMessageSend(m.ChannelID, "Current Daily Benchmark: "+decimal.NewFromFloat(dailyBmEntry.ROIValue).String()+"%")
}

func (serviceManager *ServiceManager) handleApproveParticipantCommand(s *discordgo.Session, m *discordgo.MessageCreate) {

	chanCreate, chanCreateError := s.UserChannelCreate(m.Author.ID)

	if chanCreateError != nil {
		logrus.Error(chanCreateError)
		return
	}

	if serviceManager.DatabaseClient.CheckIfParticipantExist(m.Author.ID) != true {
		s.ChannelMessageSend(chanCreate.ID, "You are not registered")
		return
	}

	participant, participantError := serviceManager.DatabaseClient.GetParticipant(m.Author.ID)

	if participantError != nil {
		logrus.Error(participantError)
		s.ChannelMessageSend(chanCreate.ID, "Something broke tell the owner you can't get your id")
		return
	}

	if participant.Approved != true {
		s.ChannelMessageSend(chanCreate.ID, "You need to be approved by another user.")
		return
	}

	contentSplit := strings.Split(m.Content, " ")

	if len(contentSplit) != 2 {
		s.ChannelMessageSend(chanCreate.ID, "Command incorrect ex. !approve <uuid>")
		return
	}

	if serviceManager.DatabaseClient.CheckIfParticipantExistByUUID(contentSplit[1]) != true {
		s.ChannelMessageSend(chanCreate.ID, "User does not exist")
		return
	}

	serviceManager.DatabaseClient.ApproveParticipantByUUID(contentSplit[1], participant.UUID)

	s.ChannelMessageSend(chanCreate.ID, "User approved")

}

func (serviceManager *ServiceManager) updateDailyBmEntry(newValue float64) {

	dailyBmEntry, dailyBmEntryError := serviceManager.DatabaseClient.GetDailyBmForToday()

	if dailyBmEntryError != nil {
		serviceManager.DatabaseClient.CreateDailyBmEntry(newValue)
		return
	}

	allTodayRoiEntries, roiEntriesError := serviceManager.DatabaseClient.GetRoiEntriesForToday()

	if roiEntriesError != nil {
		logrus.Error(roiEntriesError)
		return
	}

	if len(allTodayRoiEntries) < 4 {
		// We do not have enough to drop the highest and lowest we just average normally
		currentValue := decimal.NewFromFloat(0.0)

		for _, element := range allTodayRoiEntries {
			currentValue = currentValue.Add(decimal.NewFromFloat(element.ROIValue))
		}

		newValue, _ := currentValue.Div(decimal.NewFromInt(int64(len(allTodayRoiEntries)))).Round(3).Float64()

		updateError := serviceManager.DatabaseClient.UpdateDailyBmEntry(dailyBmEntry.UUID, newValue)

		if updateError != nil {
			logrus.Error(updateError)
			return
		}

		return
	}

	// Calculate basic index style (Drop the highest and lowest and average the remainder

	sort.SliceStable(allTodayRoiEntries, func(i, j int) bool {
		return allTodayRoiEntries[i].ROIValue < allTodayRoiEntries[j].ROIValue
	})

	// Remove the first entry
	allTodayRoiEntries = allTodayRoiEntries[1:]

	// Remove the last entry
	allTodayRoiEntries = allTodayRoiEntries[:len(allTodayRoiEntries)-1]

	// Average out and save the value
	currentValue := decimal.NewFromFloat(0.0)

	for _, element := range allTodayRoiEntries {
		currentValue = currentValue.Add(decimal.NewFromFloat(element.ROIValue))
	}

	newValueRound, _ := currentValue.Div(decimal.NewFromInt(int64(len(allTodayRoiEntries)))).Round(3).Float64()

	updateError := serviceManager.DatabaseClient.UpdateDailyBmEntry(dailyBmEntry.UUID, newValueRound)

	if updateError != nil {
		logrus.Error(updateError)
		return
	}

}

func (serviceManager *ServiceManager) handleShowLeaderBoardCommand(s *discordgo.Session, m *discordgo.MessageCreate) {

	chanCreate, chanCreateError := s.UserChannelCreate(m.Author.ID)

	if chanCreateError != nil {
		logrus.Error(chanCreateError)
		return
	}

	if serviceManager.DatabaseClient.CheckIfParticipantExist(m.Author.ID) != true {
		s.ChannelMessageSend(chanCreate.ID, "You are not registered")
		return
	}

	participant, participantError := serviceManager.DatabaseClient.GetParticipant(m.Author.ID)

	if participantError != nil {
		logrus.Error(participantError)
		s.ChannelMessageSend(chanCreate.ID, "Something broke tell the owner you can't get your id")
		return
	}

	if participant.Approved != true {
		s.ChannelMessageSend(chanCreate.ID, "You need to be approved by another user.")
		return
	}

	allTodayRoiEntries, roiEntriesError := serviceManager.DatabaseClient.GetRoiEntriesForToday()

	if roiEntriesError != nil {
		logrus.Error(roiEntriesError)
		s.ChannelMessageSend(m.ChannelID, "Failed to retrieve all roi entries")
		return
	}

	sort.SliceStable(allTodayRoiEntries, func(i, j int) bool {
		return allTodayRoiEntries[i].ROIValue < allTodayRoiEntries[j].ROIValue
	})

	tableData := [][]string{}

	for _, entry := range allTodayRoiEntries {

		nameToShow := entry.ParticipantUUID[:8]
		participantModel, participantModelError := serviceManager.DatabaseClient.GetParticipantByUUID(entry.ParticipantUUID)

		if participantModelError == nil {
			if participantModel.ShowNameInLeaderboard == true {
				nameToShow = participantModel.Username
			}
		}

		tableData = append(tableData, []string{nameToShow, entry.SubmissionTime.Format(time.RFC822), decimal.NewFromFloat(entry.ROIValue).String()})
	}

	tableString := &strings.Builder{}
	table := tablewriter.NewWriter(tableString)

	table.SetHeader([]string{"Participant", "Time", "ROI%"})

	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	for _, v := range tableData {
		table.Append(v)
	}

	table.Render()
	s.ChannelMessageSend(m.ChannelID, "Current Leader board\n```"+tableString.String()+"```")
}

func (serviceManager *ServiceManager) handleToggleLeaderboardCommand(s *discordgo.Session, m *discordgo.MessageCreate) {

	chanCreate, chanCreateError := s.UserChannelCreate(m.Author.ID)

	if chanCreateError != nil {
		logrus.Error(chanCreateError)
		return
	}

	if serviceManager.DatabaseClient.CheckIfParticipantExist(m.Author.ID) != true {
		s.ChannelMessageSend(chanCreate.ID, "You are not registered")
		return
	}

	participantModel, participantError := serviceManager.DatabaseClient.GetParticipant(m.Author.ID)

	if participantError != nil {
		logrus.Error(participantError)
		s.ChannelMessageSend(chanCreate.ID, "Something broke tell the owner you can't get your id")
	}

	if participantModel.ShowNameInLeaderboard == false {
		serviceManager.DatabaseClient.ShowNameInLeaderboardParticipantByUUID(participantModel.UUID, true)
		s.ChannelMessageSend(chanCreate.ID, "Your name will now show up in the leaderboard")
		return
	}

	serviceManager.DatabaseClient.ShowNameInLeaderboardParticipantByUUID(participantModel.UUID, false)
	s.ChannelMessageSend(chanCreate.ID, "Your name will now be hidden in the leaderboard")
	return

}

func (serviceManager *ServiceManager) messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {

	// Ignore all messages created by the bot itself
	// This isn't required in this specific example but it's a good practice.
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Uncomment to see messages incoming to the bot
	//logrus.Debug("Got message from user: " + m.Author.ID + " - " + m.Content)

	if m.Content == "!register" {
		serviceManager.handleRegisterCommand(s, m)
	} else if m.Content == "!giveInfo" {
		serviceManager.handleGiveInfoCommand(s, m)
	} else if strings.HasPrefix(m.Content, "!submitRoi") {
		serviceManager.handleSubmitRoiCommand(s, m)
	} else if m.Content == "!dailyBm" {
		serviceManager.handleDailyBmCommand(s, m)
	} else if m.Content == "!showLeaderBoard" {
		serviceManager.handleShowLeaderBoardCommand(s, m)
	} else if strings.HasPrefix(m.Content, "!approve") {
		serviceManager.handleApproveParticipantCommand(s, m)
	} else if m.Content == "!toggleLeaderBoard" {
		serviceManager.handleToggleLeaderboardCommand(s, m)
	}

}

func (serviceManager *ServiceManager) Shutdown() {
	serviceManager.DiscordClient.Close()
}
