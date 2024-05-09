all:
	go build ./cmd/rdssnap/
	go build ./cmd/rdsreceive/
	go build ./cmd/snapshotizer/
	go build ./cmd/receivesnaps/

clean:
	 rm -f rdssnap rdsreceive snapshotizer receivesnaps snapshot_ids.txt cluster_snapshots.txt instance_snapshots.txt
