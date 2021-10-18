package subcommand

import (
	"go-swan-client/common/client"
	"go-swan-client/common/constants"
	"go-swan-client/common/utils"
	"go-swan-client/config"
	"go-swan-client/logs"
	"go-swan-client/model"
)

func SendAutoBidDeal(outputDir *string) {
	swanClient := client.SwanGetClient()
	assignedTasks, err := swanClient.GetAssignedTasks()
	if err != nil {
		logs.GetLogger().Error(err)
		return
	}
	logs.GetLogger().Info("autobid Swan task count:", len(assignedTasks))
	if len(assignedTasks) == 0 {
		logs.GetLogger().Info("no autobid task to be dealt with")
		return
	}

	for _, assignedTask := range assignedTasks {
		assignedTaskInfo, err := swanClient.GetOfflineDealsByTaskUuid(assignedTask.Uuid)
		if err != nil {
			logs.GetLogger().Error(err)
			continue
		}

		deals := assignedTaskInfo.Data.Deal
		miner := assignedTaskInfo.Data.Miner
		task := assignedTaskInfo.Data.Task
		csvFilePath, err := SendAutobidDeal(deals, miner, task, outputDir)

		if err != nil {
			logs.GetLogger().Error(err)
			continue
		}

		response := swanClient.UpdateAssignedTask(assignedTask.Uuid, csvFilePath)
		logs.GetLogger().Info(response)
	}
}

func SendAutobidDeal(deals []model.OfflineDeal, miner model.Miner, task model.Task, outputDir *string) (string, error) {
	carFiles := []*model.FileDesc{}

	for _, deal := range deals {
		dealConfig := GetDealConfig4Autobid(task, deal)
		err := CheckDealConfig(dealConfig)
		if err != nil {
			logs.GetLogger().Error(err)
			continue
		}

		fileSizeInt := utils.GetInt64FromStr(*deal.FileSize)
		if fileSizeInt <= 0 {
			logs.GetLogger().Error("file is too small")
			continue
		}
		pieceSize, sectorSize := CalculatePieceSize(fileSizeInt)
		logs.GetLogger().Info("dealConfig.MinerPrice:", dealConfig.MinerPrice)
		cost := CalculateRealCost(sectorSize, dealConfig.MinerPrice)
		carFile := model.FileDesc{
			StartEpoch: *deal.StartEpoch,
			PieceCid:   *deal.PieceCid,
			DataCid:    *deal.PayloadCid,
		}
		carFiles = append(carFiles, &carFile)
		dealCid, err := client.LotusProposeOfflineDeal(carFile, cost, pieceSize, *dealConfig)
		if err != nil {
			logs.GetLogger().Error(err)
			continue
		}
		if dealCid == nil {
			continue
		}
		carFile.MinerFid = task.MinerFid
		carFile.DealCid = *dealCid
	}

	if outputDir == nil {
		outDir := config.GetConfig().Sender.OutputDir
		outputDir = &outDir
	}

	jsonFileName := task.TaskName + constants.JSON_FILE_NAME_BY_AUTO
	csvFileName := task.TaskName + constants.CSV_FILE_NAME_BY_AUTO
	WriteCarFilesToFiles(carFiles, *outputDir, jsonFileName, csvFileName)

	csvFilename := task.TaskName + "_deal.csv"
	csvFilepath, err := CreateCsv4TaskDeal(carFiles, task.MinerFid, *outputDir, csvFilename)

	return csvFilepath, err
}

func GetDealConfig4Autobid(task model.Task, deal model.OfflineDeal) *model.DealConfig {
	dealConfig := model.DealConfig{
		MinerFid:           *task.MinerFid,
		SenderWallet:       config.GetConfig().Sender.Wallet,
		VerifiedDeal:       *task.Type == constants.TASK_TYPE_VERIFIED,
		FastRetrieval:      *task.FastRetrieval == constants.TASK_FAST_RETRIEVAL,
		EpochIntervalHours: config.GetConfig().Sender.StartEpochHours,
		SkipConfirmation:   config.GetConfig().Sender.SkipConfirmation,
		StartEpochHours:    *deal.StartEpoch,
	}

	dealConfig.MaxPrice = *task.MaxPrice

	return &dealConfig
}
