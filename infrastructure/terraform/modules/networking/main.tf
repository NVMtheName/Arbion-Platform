variable "name" {

  type = string
}
variable "vpc_cidr" {

  type = string
}
variable "availability_zones" {

  type = list(string)
}
variable "nat_gateway_per_az" {

  type    = bool
  default = true
}
resource "aws_vpc" "this" {

  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags = {
    Name = var.name
  }
}
resource "aws_internet_gateway" "this" {

  vpc_id = aws_vpc.this.id
}
resource "aws_subnet" "public" {

  count                   = length(var.availability_zones)
  vpc_id                  = aws_vpc.this.id
  availability_zone       = var.availability_zones[count.index]
  cidr_block              = cidrsubnet(var.vpc_cidr, 4, count.index)
  map_public_ip_on_launch = false
  tags = {
    Name = "${var.name}-public-${count.index + 1}"
  }
}
resource "aws_subnet" "app" {

  count             = length(var.availability_zones)
  vpc_id            = aws_vpc.this.id
  availability_zone = var.availability_zones[count.index]
  cidr_block        = cidrsubnet(var.vpc_cidr, 4, count.index + 4)
  tags = {
    Name = "${var.name}-app-${count.index + 1}"
  }
}
resource "aws_subnet" "data" {

  count             = length(var.availability_zones)
  vpc_id            = aws_vpc.this.id
  availability_zone = var.availability_zones[count.index]
  cidr_block        = cidrsubnet(var.vpc_cidr, 4, count.index + 8)
  tags = {
    Name = "${var.name}-data-${count.index + 1}"
  }
}
resource "aws_route_table" "public" {

  vpc_id = aws_vpc.this.id
  route {

    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }
}
resource "aws_route_table_association" "public" {

  count          = length(aws_subnet.public)
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}
resource "aws_eip" "nat" {

  count      = var.nat_gateway_per_az ? length(var.availability_zones) : 1
  domain     = "vpc"
  depends_on = [aws_internet_gateway.this]
}
resource "aws_nat_gateway" "this" {

  count         = length(aws_eip.nat)
  allocation_id = aws_eip.nat[count.index].id
  subnet_id     = aws_subnet.public[count.index].id
  depends_on    = [aws_internet_gateway.this]
}
resource "aws_route_table" "app" {

  count  = length(var.availability_zones)
  vpc_id = aws_vpc.this.id
  route {

    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.this[var.nat_gateway_per_az ? count.index : 0].id
  }
}
resource "aws_route_table_association" "app" {

  count          = length(aws_subnet.app)
  subnet_id      = aws_subnet.app[count.index].id
  route_table_id = aws_route_table.app[count.index].id
}
resource "aws_route_table" "data" {

  count  = length(var.availability_zones)
  vpc_id = aws_vpc.this.id
}
resource "aws_route_table_association" "data" {

  count          = length(aws_subnet.data)
  subnet_id      = aws_subnet.data[count.index].id
  route_table_id = aws_route_table.data[count.index].id
}
output "vpc_id" {

  value = aws_vpc.this.id
}
output "public_subnet_ids" {

  value = aws_subnet.public[*].id
}
output "app_subnet_ids" {

  value = aws_subnet.app[*].id
}
output "data_subnet_ids" {

  value = aws_subnet.data[*].id
}
