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
	"github.com/aws/aws-sdk-go/service/sts"
)

func getRDSInstances(svc *rds.RDS) ([]*rds.DBInstance, error) {
	instances := []*rds.DBInstance{}
	err := svc.DescribeDBInstancesPages(
		&rds.DescribeDBInstancesInput{},
		func(page *rds.DescribeDBInstancesOutput, lastPage bool) bool {
			instances = append(instances, page.DBInstances...)
			return !lastPage
		},
	)
	return instances, err
}

func getRDSClusters(svc *rds.RDS) ([]*rds.DBCluster, error) {
	instances := []*rds.DBCluster{}
	err := svc.DescribeDBClustersPages(
		&rds.DescribeDBClustersInput{},
		func(page *rds.DescribeDBClustersOutput, lastPage bool) bool {
			instances = append(instances, page.DBClusters...)
			return !lastPage
		},
	)
	return instances, err
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

func isInCluster(instance *rds.DBInstance) bool {
	return instance.DBClusterIdentifier != nil
}

func snapshotReady(svc *rds.RDS, name string) bool {
	res, err := svc.DescribeDBSnapshots(&rds.DescribeDBSnapshotsInput{
		Filters: []*rds.Filter{
			{
				Name:   aws.String("db-snapshot-id"),
				Values: aws.StringSlice([]string{name}),
			},
		},
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
	res, err := svc.DescribeDBSnapshots(&rds.DescribeDBSnapshotsInput{
		SnapshotType: aws.String("manual"),
	})
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

var MAX_SNAPSHOT_COPIES int = 10

func main() {
	//args key, account shared
	KMSArn := os.Args[1]
	TargetAccountId := os.Args[2]

	if sc, err := strconv.Atoi(os.Getenv("MAX_SNAPSHOT_COPIES")); err == nil {
		MAX_SNAPSHOT_COPIES = sc
	}
	fmt.Println(MAX_SNAPSHOT_COPIES)

	sess := session.Must(session.NewSessionWithOptions(
		session.Options{
			SharedConfigState: session.SharedConfigEnable,
		},
	))
	stssvc := sts.New(sess)
	res, err := stssvc.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		fmt.Println("failed to get caller identity", err)
		os.Exit(1)
	}
	AccountId := aws.StringValue(res.Account)
	fmt.Println("Running from Account:", AccountId)

	svc := rds.New(sess)

	instances, err := getRDSInstances(svc)
	if err != nil {
		fmt.Println("Failed to query rds instances:", err)
	}

	clusters, err := getRDSClusters(svc)
	if err != nil {
		fmt.Println("Failed to query cluster instances")
	}

	InstanceSnapshotsCreated := []string{}

	//################################################################################
	//# Create intermediate snapshots                                                #
	//################################################################################

	// Clusters and Instances are snapshotted differently, need to handle in different loops
	fmt.Println("Creating Intermediate Snapshots")
	for _, instance := range instances {
		fmt.Println(aws.StringValue(instance.DBInstanceIdentifier))
		if isInCluster(instance) {
			fmt.Println("    Database Instance is in cluster, skipping")
			continue
		}
		//fmt.Println(instance)

		//snapshot ids are unique, and trying to create a new snapshot with the same name is not allowed.
		//need to destroy the old snapshot first
		IntermediateSnapshotName := fmt.Sprintf("inter-%s", aws.StringValue(instance.DBInstanceIdentifier))
		err = findAndDestroyOldSnapshot(svc, IntermediateSnapshotName)
		if err != nil {
			fmt.Printf("    Failed to find and destroy snapshot %s: %s\n", IntermediateSnapshotName, err)
			continue
		}
		waitOnConcurrentSnapshots(svc)
		res, err := svc.CreateDBSnapshot(&rds.CreateDBSnapshotInput{
			DBInstanceIdentifier: instance.DBInstanceIdentifier,
			DBSnapshotIdentifier: aws.String(IntermediateSnapshotName),
		})
		if err != nil {
			fmt.Println("   Failed to create snapshot:", err)
			continue
		}
		fmt.Println("    Snapshot", aws.StringValue(res.DBSnapshot.DBSnapshotIdentifier), "created")
		InstanceSnapshotsCreated = append(InstanceSnapshotsCreated, aws.StringValue(res.DBSnapshot.DBSnapshotIdentifier))
	}

	ClusterSnapshotsCreated := []string{}

	for _, cluster := range clusters {
		clusterName := aws.StringValue(cluster.DBClusterIdentifier)
		fmt.Println(clusterName)
		IntermediateSnapshotName := fmt.Sprintf("inter-%s", clusterName)

		err = findAndDestroyOldClusterSnapshot(svc, IntermediateSnapshotName)
		if err != nil {
			fmt.Printf("   Failed to find and destroy cluster snapshot %s: %s\n", IntermediateSnapshotName, err)
			continue
		}

		waitOnConcurrentSnapshots(svc)
		res, err := svc.CreateDBClusterSnapshot(&rds.CreateDBClusterSnapshotInput{
			DBClusterIdentifier:         aws.String(clusterName),
			DBClusterSnapshotIdentifier: aws.String(IntermediateSnapshotName),
		})
		if err != nil {
			fmt.Printf("   Failed to create cluster snapshot %s: %s\n", IntermediateSnapshotName, err)
		}

		fmt.Println("    Cluster snapshot", aws.StringValue(res.DBClusterSnapshot.DBClusterSnapshotIdentifier), "created")
		ClusterSnapshotsCreated = append(ClusterSnapshotsCreated, *res.DBClusterSnapshot.DBClusterSnapshotIdentifier)

	}

	fmt.Println()

	//################################################################################
	//# Copy Intermediate Snapshots to Shared snapshots                              #
	//################################################################################

	fmt.Println("Copying to Shared Snapshots")
	//Copy snapshots with new key
	SharedInstanceSnapshotsCreated := []string{}
	for _, instanceSnap := range InstanceSnapshotsCreated {
		SharedSnapshotName := strings.ReplaceAll(instanceSnap, "inter-", "shared-")
		fmt.Println(instanceSnap, "->", SharedSnapshotName)

		waitForSnapshot(svc, instanceSnap)
		findAndDestroyOldSnapshot(svc, SharedSnapshotName)
		waitOnConcurrentSnapshots(svc)

		_, err := svc.CopyDBSnapshot(&rds.CopyDBSnapshotInput{
			KmsKeyId:                   aws.String(KMSArn),
			SourceDBSnapshotIdentifier: aws.String(instanceSnap),
			TargetDBSnapshotIdentifier: aws.String(SharedSnapshotName),
		})
		if err != nil {
			fmt.Println("   Failed to initiate snapshot Copy: ", err)
			continue
		}

		fmt.Println("    Initiated snapshot copy")
		SharedInstanceSnapshotsCreated = append(SharedInstanceSnapshotsCreated, SharedSnapshotName)
	}

	SharedClusterSnapshotsCreated := []string{}
	fmt.Println("Copying to shared cluster snapshots")
	for _, clusterSnap := range ClusterSnapshotsCreated {
		SharedSnapshotName := strings.ReplaceAll(clusterSnap, "inter-", "shared-")
		fmt.Println(clusterSnap, "->", SharedSnapshotName)

		waitForClusterSnapshot(svc, clusterSnap)
		findAndDestroyOldClusterSnapshot(svc, SharedSnapshotName)
		waitOnConcurrentSnapshots(svc)

		_, err := svc.CopyDBClusterSnapshot(&rds.CopyDBClusterSnapshotInput{
			KmsKeyId:                          aws.String(KMSArn),
			SourceDBClusterSnapshotIdentifier: aws.String(clusterSnap),
			TargetDBClusterSnapshotIdentifier: aws.String(SharedSnapshotName),
		})
		if err != nil {
			fmt.Println("   Failed to initiate snapshot Copy: ", err)
			continue
		}

		fmt.Println("    Initiated snapshot copy")
		SharedClusterSnapshotsCreated = append(SharedClusterSnapshotsCreated, SharedSnapshotName)
	}

	fmt.Println()

	//################################################################################
	//# Share the snapshots                                                          #
	//################################################################################

	fmt.Println("Sharing snapshots to target account")

	for _, instanceSnap := range SharedInstanceSnapshotsCreated {
		fmt.Println(instanceSnap, "->", TargetAccountId)

		waitForSnapshot(svc, instanceSnap)
		waitOnConcurrentSnapshots(svc)
		_, err = svc.ModifyDBSnapshotAttribute(&rds.ModifyDBSnapshotAttributeInput{
			DBSnapshotIdentifier: aws.String(instanceSnap),
			AttributeName:        aws.String("restore"),
			ValuesToAdd: aws.StringSlice([]string{
				TargetAccountId,
			}),
		})
		if err != nil {
			fmt.Println("    Failed to share snapshot", instanceSnap, "to", TargetAccountId, "-", err)
			continue
		}
		fmt.Println("    Snapshot", instanceSnap, "shared to account", TargetAccountId)
	}

	for _, clusterSnap := range SharedClusterSnapshotsCreated {
		fmt.Println(clusterSnap, "->", TargetAccountId)
		waitForClusterSnapshot(svc, clusterSnap)
		waitOnConcurrentSnapshots(svc)
		_, err = svc.ModifyDBClusterSnapshotAttribute(&rds.ModifyDBClusterSnapshotAttributeInput{
			DBClusterSnapshotIdentifier: aws.String(clusterSnap),
			AttributeName:               aws.String("restore"),
			ValuesToAdd: aws.StringSlice([]string{
				TargetAccountId,
			}),
		})
		if err != nil {
			fmt.Println("    Failed to share snapshot", clusterSnap, "to", TargetAccountId, "-", err)
			continue
		}
		fmt.Println("    Snapshot", clusterSnap, "shared to account", TargetAccountId)
	}

	i, err := os.Create("instance_snapshots.txt")
	if err != nil {
		fmt.Println("failed to open instance snapshots file:", err)
		os.Exit(1)
	}
	defer i.Close()

	c, err := os.Create("cluster_snapshots.txt")
	if err != nil {
		fmt.Println("failed to open cluster snapshots file:", err)
		os.Exit(1)
	}
	defer c.Close()

	iw := bufio.NewWriter(i)
	cw := bufio.NewWriter(c)

	fmt.Println("Instance snapshots:")
	for _, instanceSnap := range SharedInstanceSnapshotsCreated {
		fullname := fmt.Sprintf("arn:aws-us-gov:rds:us-gov-west-1:%s:snapshot:%s", AccountId, instanceSnap)
		fmt.Fprintln(iw, fullname)
		fmt.Println(fullname)
	}

	fmt.Println("Cluster snapshots:")
	for _, clusterSnap := range SharedClusterSnapshotsCreated {
		fullname := fmt.Sprintf("arn:aws-us-gov:rds:us-gov-west-1:%s:cluster-snapshot:%s", AccountId, clusterSnap)
		fmt.Fprintln(cw, fullname)
		fmt.Println(fullname)
	}
	iw.Flush()
	cw.Flush()
}
