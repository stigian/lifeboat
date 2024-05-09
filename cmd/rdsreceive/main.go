package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
)

func snapshotReady(svc *rds.RDS, name string) bool {
	res, err := svc.DescribeDBSnapshots(&rds.DescribeDBSnapshotsInput{
		Filters: []*rds.Filter{
			{
				Name:   aws.String("db-snapshot-id"),
				Values: aws.StringSlice([]string{name}),
			},
		},
		IncludeShared: aws.Bool(true),
	})
	if err != nil {
		return false //it really shouldnt fail here
	}
	status := aws.StringValue(res.DBSnapshots[0].Status)
	return status == "available" || status == "failed"
}

func clusterSnapshotReady(svc *rds.RDS, name string) bool {
	res, err := svc.DescribeDBClusterSnapshots(&rds.DescribeDBClusterSnapshotsInput{
		Filters: []*rds.Filter{
			{
				Name:   aws.String("db-cluster-snapshot-id"),
				Values: aws.StringSlice([]string{name}),
			},
		},
		IncludeShared: aws.Bool(true),
	})
	if err != nil {
		return false //it really shouldnt fail here
	}
	status := aws.StringValue(res.DBClusterSnapshots[0].Status)
	return status == "available" || status == "failed"
}

func waitForSnapshot(svc *rds.RDS, name string) {
	waitSnap := false
	if !snapshotReady(svc, name) {
		waitSnap = true
		fmt.Printf("    Waiting on snapshot %s to become ready...", name)
	}
	for !snapshotReady(svc, name) {
		fmt.Printf(".")
		time.Sleep(2 * time.Second)
	}
	if waitSnap {
		fmt.Printf("\n")
	}
}

func waitForClusterSnapshot(svc *rds.RDS, name string) {
	waitSnap := false
	if !clusterSnapshotReady(svc, name) {
		waitSnap = true
		fmt.Printf("    Waiting on snapshot %s to become ready...", name)
	}
	for !clusterSnapshotReady(svc, name) {
		fmt.Printf(".")
		time.Sleep(2 * time.Second)
	}
	if waitSnap {
		fmt.Printf("\n")
	}
}
func numSnapshotsInProgress(svc *rds.RDS) int {
	count := 0
	res, err := svc.DescribeDBSnapshots(&rds.DescribeDBSnapshotsInput{})
	if err != nil {
		panic("cant query db snapshots")
	}
	for _, snap := range res.DBSnapshots {
		if aws.StringValue(snap.Status) == "creating" {
			count += 1
		}
	}
	return count
}

func numClusterSnapshotsInProgress(svc *rds.RDS) int {
	count := 0
	res, err := svc.DescribeDBSnapshots(&rds.DescribeDBSnapshotsInput{})
	if err != nil {
		panic("cant query db snapshots")
	}
	for _, snap := range res.DBSnapshots {
		if aws.StringValue(snap.Status) == "creating" {
			count += 1
		}
	}
	return count
}

func waitOnConcurrentSnapshots(svc *rds.RDS) {
	waiting := false
	if numSnapshotsInProgress(svc)+numClusterSnapshotsInProgress(svc) >= MAX_SNAPSHOT_COPIES {
		waiting = true
		fmt.Printf("    Max Concurrent Snapshots exceed, waiting...")
	}
	for numSnapshotsInProgress(svc)+numClusterSnapshotsInProgress(svc) >= MAX_SNAPSHOT_COPIES {
		fmt.Printf(".")
		time.Sleep(2 * time.Second)
	}
	if waiting {
		fmt.Print("\n")
	}
}
func findAndDestroyOldSnapshot(svc *rds.RDS, name string) error {
	res, err := svc.DescribeDBSnapshots(&rds.DescribeDBSnapshotsInput{
		Filters: []*rds.Filter{
			{
				Name:   aws.String("db-snapshot-id"),
				Values: aws.StringSlice([]string{name}),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to describe snapshot %s: %s", name, err)
	}
	if len(res.DBSnapshots) > 0 {
		waitForSnapshot(svc, name)
		_, err := svc.DeleteDBSnapshot(&rds.DeleteDBSnapshotInput{
			DBSnapshotIdentifier: aws.String(name),
		})
		if err != nil {
			return fmt.Errorf("failed to delete snapshot %s: %s", name, err)
		}
	}
	return nil
}

func findAndDestroyOldClusterSnapshot(svc *rds.RDS, name string) error {
	res, err := svc.DescribeDBClusterSnapshots(&rds.DescribeDBClusterSnapshotsInput{
		Filters: []*rds.Filter{
			{
				Name:   aws.String("db-cluster-snapshot-id"),
				Values: aws.StringSlice([]string{name}),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to describe snapshot %s: %s", name, err)
	}
	if len(res.DBClusterSnapshots) > 0 {
		waitForClusterSnapshot(svc, name)
		_, err := svc.DeleteDBClusterSnapshot(&rds.DeleteDBClusterSnapshotInput{
			DBClusterSnapshotIdentifier: aws.String(name),
		})
		if err != nil {
			return fmt.Errorf("failed to delete snapshot %s: %s", name, err)
		}
	}
	return nil
}

func convertSnapshotName(in string) string {
	split := strings.Split(in, ":")
	accountFrom := split[4]
	baseName := split[6]
	name := strings.ReplaceAll(baseName, "shared-", "")
	name = fmt.Sprintf("received-%s-%s", accountFrom, name)
	return name
}

var MAX_SNAPSHOT_COPIES int = 10 //default to 10, override from environment variable
func main() {
	KMSArn := os.Args[1]
	InstanceSnapshotFile := os.Args[2]
	ClusterSnapshotFile := os.Args[3]

	if sc, err := strconv.Atoi(os.Getenv("MAX_SNAPSHOT_COPIES")); err == nil {
		MAX_SNAPSHOT_COPIES = sc
	}

	instancesFile, err := os.Open(InstanceSnapshotFile)
	if err != nil {
		fmt.Println("Failed to open snapshots file:", err)
		os.Exit(1)
	}
	defer instancesFile.Close()
	InstanceScanner := bufio.NewScanner(instancesFile)

	clusterFile, err := os.Open(ClusterSnapshotFile)
	if err != nil {
		fmt.Println("Failed to open cluster snapshots file:", err)
		os.Exit(1)
	}
	defer clusterFile.Close()
	ClusterScanner := bufio.NewScanner(clusterFile)

	sess := session.Must(session.NewSessionWithOptions(
		session.Options{
			SharedConfigState: session.SharedConfigEnable,
		},
	))
	svc := rds.New(sess)
	//################################################################################
	//# Receive Instance Snapshots                                                   #
	//################################################################################
	for InstanceScanner.Scan() {
		snapshotid := InstanceScanner.Text()
		if snapshotid == "" {
			continue
		}
		fmt.Println("Snapshot:", snapshotid)
		//check if input snapshot is completed
		waitForSnapshot(svc, snapshotid)

		//poll on pending snapshot copies to avoid exceeding limits
		waitOnConcurrentSnapshots(svc)

		//copy snapshot
		//need to delete the arn out of the snapshot identifier
		name := convertSnapshotName(snapshotid)
		findAndDestroyOldSnapshot(svc, name)
		_, err := svc.CopyDBSnapshot(&rds.CopyDBSnapshotInput{
			KmsKeyId:                   aws.String(KMSArn),
			SourceDBSnapshotIdentifier: aws.String(snapshotid),
			TargetDBSnapshotIdentifier: aws.String(name),
		})
		if err != nil {
			fmt.Println("failed to copy snapshot: ", err)
			continue
		}
		fmt.Printf("Snapshot copy initiated: %s -> %s \n", snapshotid, name)
		fmt.Println()
	}

	//################################################################################
	//# Receive Cluster Snapshots                                                    #
	//################################################################################
	for ClusterScanner.Scan() {
		clusterid := ClusterScanner.Text()
		if clusterid == "" {
			continue
		}
		fmt.Println("Cluster Snapshot:", clusterid)
		waitForClusterSnapshot(svc, clusterid)
		waitOnConcurrentSnapshots(svc)
		name := convertSnapshotName(clusterid)
		findAndDestroyOldClusterSnapshot(svc, name)
		_, err := svc.CopyDBClusterSnapshot(&rds.CopyDBClusterSnapshotInput{
			KmsKeyId:                          aws.String(KMSArn),
			SourceDBClusterSnapshotIdentifier: aws.String(clusterid),
			TargetDBClusterSnapshotIdentifier: aws.String(name),
		})
		if err != nil {
			fmt.Println("failed to copy snapshot: ", err)
			continue
		}
		fmt.Printf("Snapshot copy initiated: %s -> %s \n", clusterid, name)
		fmt.Println()
	}
	fmt.Println("Complete")
}
